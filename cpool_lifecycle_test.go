package grpcx_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

func TestClientConnPool_IdleAndConnectingConnectionsAreUsable(t *testing.T) {
	for _, state := range []connectivity.State{connectivity.Idle, connectivity.Connecting, connectivity.Ready} {
		t.Run(state.String(), func(t *testing.T) {
			dialer := newFakeDialer(func(int) connectivity.State { return state })
			pool := newFakePool(t, dialer, grpcx.WithPoolSize(4))

			runConcurrently(50, func() { require.NoError(t, invoke(pool)) })

			require.Equal(t, 4, dialer.count(), "usable connections must not be replaced")
			require.Zero(t, dialer.closed(), "usable connections must not be closed")
		})
	}
}

func TestClientConnPool_ReplacesTransientFailureOnRequestPath(t *testing.T) {
	dialer := newFakeDialer(func(n int) connectivity.State {
		if n < 2 {
			return connectivity.TransientFailure
		}

		return connectivity.Ready
	})
	pool := newFakePool(t, dialer, grpcx.WithPoolSize(2))

	require.NoError(t, invoke(pool))

	replacement := dialer.conn(2)
	require.EqualValues(t, 1, replacement.calls.Load(), "the RPC must run on the replacement")
	require.Eventually(t, func() bool { return dialer.conn(0).isClosed() || dialer.conn(1).isClosed() }, time.Second, time.Millisecond)
}

func TestClientConnPool_RefresherReplacesTransientFailure(t *testing.T) {
	dialer := newFakeDialer(func(n int) connectivity.State {
		if n < 3 {
			return connectivity.TransientFailure
		}

		return connectivity.Ready
	})
	newFakePool(t, dialer, grpcx.WithPoolSize(3), grpcx.WithPoolHealthCheckFreq(5*time.Millisecond))

	require.Eventually(t, func() bool {
		return dialer.conn(0).isClosed() && dialer.conn(1).isClosed() && dialer.conn(2).isClosed()
	}, time.Second, time.Millisecond)
	require.Equal(t, 6, dialer.count())
}

func TestClientConnPool_RefresherRotatesExpiredConnections(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
	newFakePool(t, dialer,
		grpcx.WithPoolSize(2),
		grpcx.WithPoolConnLifetime(20*time.Millisecond),
		grpcx.WithPoolJitter(0),
		grpcx.WithPoolHealthCheckFreq(5*time.Millisecond),
	)

	require.Eventually(t, func() bool { return dialer.conn(0).isClosed() && dialer.conn(1).isClosed() }, time.Second, time.Millisecond)
}

func TestClientConnPool_ConcurrentRefreshInstallsOneReplacement(t *testing.T) {
	dialer := newFakeDialer(func(n int) connectivity.State {
		if n == 0 {
			return connectivity.TransientFailure
		}

		return connectivity.Ready
	})
	pool := newFakePool(t, dialer, grpcx.WithPoolSize(1))

	runConcurrently(50, func() { require.NoError(t, invoke(pool)) })

	var served []int

	for i, c := range dialer.all() {
		if c.calls.Load() > 0 {
			served = append(served, i)
		}

		if i > 0 && c.calls.Load() == 0 {
			require.Eventually(t, c.isClosed, time.Second, time.Millisecond, "losing replacement %d must be closed", i)
		}
	}

	require.Len(t, served, 1, "all RPCs must run on the single installed replacement, got %v", served)
	require.False(t, dialer.conn(served[0]).isClosed())
}

func TestClientConnPool_NeverClosesConnectionWithRPCInFlight(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
	pool := newFakePool(t, dialer, grpcx.WithPoolSize(1))

	first := dialer.conn(0)
	first.block = make(chan struct{})

	result := make(chan error, 1)

	go func() { result <- invoke(pool) }()

	require.Eventually(t, func() bool { return first.inflight.Load() == 1 }, time.Second, time.Millisecond)

	first.state.Store(int32(connectivity.TransientFailure))
	require.NoError(t, invoke(pool), "the next RPC must run on a replacement")

	time.Sleep(20 * time.Millisecond)
	require.False(t, first.isClosed(), "replaced connection closed with an RPC in flight")

	close(first.block)
	require.NoError(t, <-result)
	require.Eventually(t, first.isClosed, time.Second, time.Millisecond)
	require.False(t, first.closedWhileBusy.Load())
}

func TestClientConnPool_DrainTimeoutClosesStuckConnection(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
	pool := newFakePool(t, dialer, grpcx.WithPoolSize(1), grpcx.WithPoolDrainTimeout(30*time.Millisecond))

	first := dialer.conn(0)
	first.block = make(chan struct{})

	result := make(chan error, 1)

	go func() { result <- invoke(pool) }()

	require.Eventually(t, func() bool { return first.inflight.Load() == 1 }, time.Second, time.Millisecond)

	first.state.Store(int32(connectivity.TransientFailure))
	require.NoError(t, invoke(pool))

	require.Equal(t, codes.Canceled, status.Code(<-result), "the stuck RPC is cancelled once the drain timeout expires")
	require.True(t, first.isClosed())
}

func TestClientConnPool_StreamKeepsConnectionOpenUntilItEnds(t *testing.T) {
	tests := map[string]func(s grpc.ClientStream, cancel context.CancelFunc){
		"recv error": func(s grpc.ClientStream, _ context.CancelFunc) { require.ErrorIs(t, s.RecvMsg(nil), io.EOF) },
		"cancelled":  func(_ grpc.ClientStream, cancel context.CancelFunc) { cancel() },
	}

	for name, end := range tests {
		t.Run(name, func(t *testing.T) {
			dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
			pool := newFakePool(t, dialer, grpcx.WithPoolSize(1))
			first := dialer.conn(0)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			stream, err := pool.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/svc/Stream")
			require.NoError(t, err)

			first.state.Store(int32(connectivity.TransientFailure))
			require.NoError(t, invoke(pool))

			time.Sleep(20 * time.Millisecond)
			require.False(t, first.isClosed(), "replaced connection closed with a stream open")

			end(stream, cancel)
			require.Eventually(t, first.isClosed, time.Second, time.Millisecond)
		})
	}
}

func TestClientConnPool_FailedNewStreamReleasesConnection(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
	pool := newFakePool(t, dialer, grpcx.WithPoolSize(1))
	first := dialer.conn(0)
	first.streamErr = errors.New("boom")

	_, err := pool.NewStream(context.Background(), &grpc.StreamDesc{}, "/svc/Stream")
	require.Error(t, err)

	first.state.Store(int32(connectivity.TransientFailure))
	require.NoError(t, invoke(pool))
	require.Eventually(t, first.isClosed, time.Second, time.Millisecond)
}

func TestClientConnPool_ConcurrentRPCsWhileConnectionsFlap(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
	dialer.delay = 200 * time.Microsecond
	pool := newFakePool(t, dialer, grpcx.WithPoolSize(3), grpcx.WithPoolHealthCheckFreq(time.Millisecond))

	stop := make(chan struct{})
	flapped := make(chan struct{})

	go func() {
		defer close(flapped)

		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
				conns := dialer.all()
				conns[i%len(conns)].state.Store(int32(connectivity.TransientFailure))
			}
		}
	}()

	runConcurrently(16, func() {
		for i := 0; i < 200; i++ {
			require.NoError(t, invoke(pool))
		}
	})

	close(stop)
	<-flapped

	require.Greater(t, dialer.count(), 3, "connections were never replaced")

	for i, c := range dialer.all() {
		require.False(t, c.closedWhileBusy.Load(), "connection %d closed with RPCs in flight", i)
	}
}

func TestClientConnPool_Close(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
	pool, err := grpcx.NewClientConnPool(dialer, grpcx.WithPoolSize(3), grpcx.WithPoolHealthCheckFreq(time.Millisecond))
	require.NoError(t, err)

	// every initial connection is replaced while an RPC is still running on
	// it, so the pool has three slots and three draining connections at Close.
	initial := dialer.all()
	block := make(chan struct{})

	defer close(block)

	for _, c := range initial {
		c.block = block
	}

	for range initial {
		go func() { _ = invoke(pool) }()
	}

	require.Eventually(t, func() bool {
		for _, c := range initial {
			if c.inflight.Load() != 1 {
				return false
			}
		}

		return true
	}, time.Second, time.Millisecond)

	for _, c := range initial {
		c.state.Store(int32(connectivity.TransientFailure))
	}

	require.Eventually(t, func() bool { return dialer.count() >= 6 }, time.Second, time.Millisecond)

	stop := make(chan struct{})

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
				}

				// RPCs started before Close are cancelled, like grpc.ClientConn.Close does.
				if err := invoke(pool); err != nil && status.Code(err) != codes.Canceled {
					require.ErrorIs(t, err, grpcx.ErrClientConnPoolClosed)
				}
			}
		}()
	}

	time.Sleep(20 * time.Millisecond)

	require.NoError(t, pool.Close())

	// calls that entered the pool before Close may still dial a replacement;
	// it must be closed right away (checked below).
	close(stop)
	wg.Wait()

	dials := dialer.count()

	require.ErrorIs(t, pool.Close(), grpcx.ErrClientConnPoolClosed)
	require.ErrorIs(t, invoke(pool), grpcx.ErrClientConnPoolClosed)
	require.Equal(t, connectivity.Shutdown, pool.GetState())

	time.Sleep(20 * time.Millisecond)
	require.Equal(t, dials, dialer.count(), "the pool dialed after Close")

	for i, c := range dialer.all() {
		require.True(t, c.isClosed(), "connection %d left open after Close", i)
		require.EqualValues(t, 1, c.closes.Load(), "connection %d closed more than once", i)
	}
}

func TestClientConnPool_GetState(t *testing.T) {
	tests := []struct {
		states []connectivity.State
		want   connectivity.State
	}{
		{[]connectivity.State{connectivity.Idle, connectivity.Ready}, connectivity.Ready},
		{[]connectivity.State{connectivity.Idle, connectivity.Connecting}, connectivity.Connecting},
		{[]connectivity.State{connectivity.TransientFailure, connectivity.Idle}, connectivity.Idle},
		{[]connectivity.State{connectivity.TransientFailure, connectivity.TransientFailure}, connectivity.TransientFailure},
	}

	for _, tt := range tests {
		t.Run(tt.want.String(), func(t *testing.T) {
			dialer := newFakeDialer(func(n int) connectivity.State { return tt.states[n%len(tt.states)] })
			pool := newFakePool(t, dialer, grpcx.WithPoolSize(len(tt.states)))

			require.Equal(t, tt.want, pool.GetState())
		})
	}
}

func TestClientConnPool_InvalidOptions(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })

	for name, opt := range map[string]grpcx.PoolOption{
		"size":              grpcx.WithPoolSize(0),
		"health check freq": grpcx.WithPoolHealthCheckFreq(0),
		"drain timeout":     grpcx.WithPoolDrainTimeout(-time.Second),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := grpcx.NewClientConnPool(dialer, opt)
			require.ErrorIs(t, err, grpcx.ErrInvalidPoolOptions)
		})
	}
}

func TestClientConnPool_DialErrorClosesDialedConnections(t *testing.T) {
	dialer := newFakeDialer(func(int) connectivity.State { return connectivity.Ready })
	dialer.failAt = 2

	_, err := grpcx.NewClientConnPool(dialer, grpcx.WithPoolSize(4))
	require.Error(t, err)

	for i, c := range dialer.all() {
		require.True(t, c.isClosed(), "connection %d leaked after a failed pool creation", i)
	}
}

func newFakePool(t *testing.T, dialer *fakeDialer, opts ...grpcx.PoolOption) grpcx.ClientConn {
	t.Helper()

	pool, err := grpcx.NewClientConnPool(dialer, opts...)
	require.NoError(t, err)

	t.Cleanup(func() { _ = pool.Close() })

	return pool
}

func invoke(cc grpc.ClientConnInterface) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return cc.Invoke(ctx, "/svc/Method", nil, nil)
}

func runConcurrently(n int, fn func()) {
	var wg sync.WaitGroup

	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start
			fn()
		}()
	}

	close(start)
	wg.Wait()
}

// fakeDialer creates fakeConns whose initial state is given by stateOf(n),
// n being the dial number.
type fakeDialer struct {
	stateOf func(n int) connectivity.State
	failAt  int           // dial number that fails, 0 = never
	delay   time.Duration // duration of every RPC

	mu    sync.Mutex
	conns []*fakeConn
}

func newFakeDialer(stateOf func(n int) connectivity.State) *fakeDialer {
	return &fakeDialer{stateOf: stateOf}
}

func (d *fakeDialer) Dial() (grpcx.ClientConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.failAt > 0 && len(d.conns) == d.failAt {
		return nil, errors.New("dial failed")
	}

	c := &fakeConn{closing: make(chan struct{}), delay: d.delay}
	c.state.Store(int32(d.stateOf(len(d.conns))))
	d.conns = append(d.conns, c)

	return c, nil
}

func (d *fakeDialer) conn(i int) *fakeConn {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.conns[i]
}

func (d *fakeDialer) all() []*fakeConn {
	d.mu.Lock()
	defer d.mu.Unlock()

	return append([]*fakeConn(nil), d.conns...)
}

func (d *fakeDialer) count() int {
	return len(d.all())
}

func (d *fakeDialer) closed() int {
	n := 0

	for _, c := range d.all() {
		if c.isClosed() {
			n++
		}
	}

	return n
}

// errConnClosing is what grpc.ClientConn returns for RPCs cancelled by Close.
var errConnClosing = status.Error(codes.Canceled, "grpc: the client connection is closing")

// fakeConn behaves like a grpc.ClientConn for the pool: RPCs block on block
// (when set) and are cancelled with errConnClosing when it is closed.
type fakeConn struct {
	state     atomic.Int32
	delay     time.Duration
	block     chan struct{}
	streamErr error

	calls           atomic.Int64
	inflight        atomic.Int64
	closes          atomic.Int64
	closedWhileBusy atomic.Bool
	closing         chan struct{}
	closeOnce       sync.Once
}

func (c *fakeConn) GetState() connectivity.State {
	return connectivity.State(c.state.Load())
}

func (c *fakeConn) Invoke(ctx context.Context, _ string, _ any, _ any, _ ...grpc.CallOption) error {
	if c.isClosed() {
		return errConnClosing
	}

	c.calls.Add(1)
	c.inflight.Add(1)
	defer c.inflight.Add(-1)

	if c.block == nil {
		select {
		case <-time.After(c.delay):
			return nil
		case <-c.closing:
			return errConnClosing
		}
	}

	select {
	case <-c.block:
		return nil
	case <-c.closing:
		return errConnClosing
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *fakeConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	if c.streamErr != nil {
		return nil, c.streamErr
	}

	return &fakeStream{}, nil
}

func (c *fakeConn) Close() error {
	if c.inflight.Load() > 0 {
		c.closedWhileBusy.Store(true)
	}

	c.closes.Add(1)
	c.closeOnce.Do(func() { close(c.closing) })

	return nil
}

func (c *fakeConn) isClosed() bool {
	select {
	case <-c.closing:
		return true
	default:
		return false
	}
}

type fakeStream struct {
	grpc.ClientStream
}

func (*fakeStream) RecvMsg(any) error { return io.EOF }
