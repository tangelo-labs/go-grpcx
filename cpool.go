package grpcx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
)

// PoolOption is a function that modifies the pool options.
type PoolOption func(*PoolOptions)

// PoolDialer is responsible for creating a new connection for the pool.
type PoolDialer interface {
	Dial() (ClientConn, error)
}

// PoolDialerFunc is a function type that implements the PoolDialer interface.
type PoolDialerFunc func() (ClientConn, error)

// Dial implements the PoolDialer interface for PoolDialerFunc.
func (p PoolDialerFunc) Dial() (ClientConn, error) {
	return p()
}

// PoolOptions holds the options for the connection pool.
type PoolOptions struct {
	poolSize        int
	connLifetime    time.Duration
	jitter          time.Duration
	healthCheckFreq time.Duration
	drainTimeout    time.Duration
}

// computeConnLifeTime returns a randomized connection lifetime duration.
func (o *PoolOptions) computeConnLifetime() time.Duration {
	if o.connLifetime <= 0 {
		return 0
	}

	rd := rand.New(rand.NewSource(time.Now().UnixNano())).Float64() + 0.5

	return time.Duration(float64(o.connLifetime) + (rd * float64(o.jitter)))
}

// WithPoolSize sets the number of connections in the pool.
func WithPoolSize(size int) PoolOption {
	return func(o *PoolOptions) {
		o.poolSize = size
	}
}

// WithPoolConnLifetime sets the lifetime of each connection in the pool.
// Defaults to 0, which means connections will not be closed automatically.
func WithPoolConnLifetime(lifetime time.Duration) PoolOption {
	return func(o *PoolOptions) {
		o.connLifetime = lifetime
	}
}

// WithPoolJitter is a random duration used to prevent from all connections
// being closed at the same time when the connection lifetime is set.
//
// This option has no effect if the connection lifetime is not set.
// Defaults to 10 seconds.
func WithPoolJitter(jitter time.Duration) PoolOption {
	return func(o *PoolOptions) {
		o.jitter = jitter
	}
}

// WithPoolHealthCheckFreq sets the frequency of health checks for the connections
// in the pool. This is used to refresh connections that are not healthy.
// Defaults to 1 minute.
func WithPoolHealthCheckFreq(freq time.Duration) PoolOption {
	return func(o *PoolOptions) {
		o.healthCheckFreq = freq
	}
}

// WithPoolDrainTimeout bounds how long a replaced connection is kept open
// for the RPCs and streams still running on it. The connection is closed as
// soon as they finish, or when this timeout expires, whichever comes first.
// Defaults to 1 minute.
func WithPoolDrainTimeout(timeout time.Duration) PoolOption {
	return func(o *PoolOptions) {
		o.drainTimeout = timeout
	}
}

// ErrClientConnPoolClosed is returned when the connection pool is closed
// and an operation is attempted on it.
var ErrClientConnPoolClosed = errors.New("grpc conn pool is closed")

// ErrInvalidPoolOptions is returned by NewClientConnPool when an option has
// a value the pool cannot work with.
var ErrInvalidPoolOptions = errors.New("invalid grpc conn pool options")

// ClientConn is an abstraction for grpc.ClientConn.
type ClientConn interface {
	GetState() connectivity.State

	grpc.ClientConnInterface
	io.Closer
}

// pooledConn is one underlying connection plus the count of RPCs and streams
// running on it, so that a replaced connection is closed only once they end.
type pooledConn struct {
	id       string
	cc       ClientConn
	deadline time.Time

	mu       sync.Mutex
	inflight int
	retired  bool
	drain    *time.Timer

	closeOnce sync.Once
	onClose   func(*pooledConn)
}

// usable reports whether RPCs can be sent on the connection. IDLE and
// CONNECTING are usable: gRPC connects on the first RPC and queues RPCs while
// connecting. Only TRANSIENT_FAILURE and SHUTDOWN need a new connection.
func (p *pooledConn) usable() bool {
	switch p.cc.GetState() {
	case connectivity.TransientFailure, connectivity.Shutdown:
		return false
	default:
		return true
	}
}

func (p *pooledConn) expired(now time.Time) bool {
	return !p.deadline.IsZero() && now.After(p.deadline)
}

// acquire registers an RPC or stream on the connection. The caller must hold
// the lock of the slot the connection belongs to, which guarantees the
// connection is not retired yet.
func (p *pooledConn) acquire() {
	p.mu.Lock()
	p.inflight++
	p.mu.Unlock()
}

func (p *pooledConn) release() {
	p.mu.Lock()
	p.inflight--
	idle := p.retired && p.inflight == 0
	p.mu.Unlock()

	if idle {
		p.close()
	}
}

// retire closes the connection once its RPCs and streams end, or after the
// drain timeout.
func (p *pooledConn) retire(drainTimeout time.Duration) {
	p.mu.Lock()
	p.retired = true
	idle := p.inflight == 0

	if !idle {
		p.drain = time.AfterFunc(drainTimeout, p.close)
	}
	p.mu.Unlock()

	if idle {
		go p.close()
	}
}

func (p *pooledConn) close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		if p.drain != nil {
			p.drain.Stop()
		}
		p.mu.Unlock()

		if err := p.cc.Close(); err != nil {
			log.Printf("%s: grpc conn pool warning, failed to close connection %s", err, p.id)
		}

		if p.onClose != nil {
			p.onClose(p)
		}
	})
}

// clientConn is a slot of the pool. The connection it holds is replaced when
// it fails or expires.
type clientConn struct {
	mu     sync.Mutex
	cur    *pooledConn
	closed bool
}

// acquire returns the slot connection with an RPC registered on it, or nil
// when the slot is closed or, if onlyUsable is set, the connection is not
// usable.
func (c *clientConn) acquire(onlyUsable bool) *pooledConn {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || (onlyUsable && !c.cur.usable()) {
		return nil
	}

	c.cur.acquire()

	return c.cur
}

func (c *clientConn) current() *pooledConn {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.cur
}

type connPoolRoundRobin struct {
	opts   *PoolOptions
	dialer PoolDialer

	slots []*clientConn
	next  atomic.Uint64

	// draining holds replaced connections that still have RPCs in flight,
	// so Close can close them too.
	draining map[*pooledConn]struct{}
	dmu      sync.Mutex

	// bmu serializes the refresher ticks with Close.
	bmu sync.Mutex

	closed      atomic.Bool
	closeSignal chan struct{}
}

// NewClientConnPool returns a new instance of ClientConn that uses a pool of
// client connections to the backend. The pool is created with the given
// dialer and options.
//
// The pool size is determined by the WithPoolSize option.
//
// Each RPC is sent on the next connection (round-robin) that is not in
// TRANSIENT_FAILURE or SHUTDOWN. A connection is replaced when it is in one
// of those states, or when its lifetime expires; the replaced connection is
// closed once the RPCs and streams running on it end (see
// WithPoolDrainTimeout).
func NewClientConnPool(dialer PoolDialer, o ...PoolOption) (ClientConn, error) {
	opts := &PoolOptions{
		poolSize:        10,
		connLifetime:    0,
		jitter:          10 * time.Second,
		healthCheckFreq: time.Minute,
		drainTimeout:    time.Minute,
	}

	for _, opt := range o {
		opt(opts)
	}

	if opts.poolSize <= 0 {
		return nil, fmt.Errorf("%w: pool size must be greater than 0, got %d", ErrInvalidPoolOptions, opts.poolSize)
	}

	if opts.healthCheckFreq <= 0 {
		return nil, fmt.Errorf("%w: health check frequency must be greater than 0, got %s", ErrInvalidPoolOptions, opts.healthCheckFreq)
	}

	if opts.drainTimeout <= 0 {
		return nil, fmt.Errorf("%w: drain timeout must be greater than 0, got %s", ErrInvalidPoolOptions, opts.drainTimeout)
	}

	pool := &connPoolRoundRobin{
		dialer:      dialer,
		opts:        opts,
		draining:    make(map[*pooledConn]struct{}),
		closeSignal: make(chan struct{}),
	}

	conns := make(chan *pooledConn, opts.poolSize)
	g := multierror.Group{}

	for i := 0; i < opts.poolSize; i++ {
		g.Go(func() error {
			w, err := pool.dial()
			if err != nil {
				return err
			}

			conns <- w

			return nil
		})
	}

	err := g.Wait().ErrorOrNil()

	close(conns)

	if err != nil {
		for conn := range conns {
			conn.close()
		}

		return nil, err
	}

	for conn := range conns {
		pool.slots = append(pool.slots, &clientConn{cur: conn})
	}

	go pool.refresher()

	return pool, nil
}

func (pool *connPoolRoundRobin) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	conn, err := pool.get()
	if err != nil {
		return fmt.Errorf("%w: failed to get a connection from the pool", err)
	}

	defer conn.release()

	return conn.cc.Invoke(ctx, method, args, reply, opts...)
}

func (pool *connPoolRoundRobin) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	conn, err := pool.get()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get a connection from the pool", err)
	}

	stream, err := conn.cc.NewStream(ctx, desc, method, opts...)
	if err != nil {
		conn.release()

		return nil, err
	}

	return newPooledStream(ctx, stream, conn.release), nil
}

// GetState returns READY if any connection is ready, else CONNECTING, IDLE
// or TRANSIENT_FAILURE in that order, and SHUTDOWN once the pool is closed.
func (pool *connPoolRoundRobin) GetState() connectivity.State {
	if pool.closed.Load() {
		return connectivity.Shutdown
	}

	seen := make(map[connectivity.State]bool, 4)
	for _, slot := range pool.slots {
		seen[slot.current().cc.GetState()] = true
	}

	for _, s := range []connectivity.State{connectivity.Ready, connectivity.Connecting, connectivity.Idle, connectivity.TransientFailure} {
		if seen[s] {
			return s
		}
	}

	return connectivity.Shutdown
}

// Close closes every connection of the pool, cancelling the RPCs and streams
// still running on them, like grpc.ClientConn.Close does.
func (pool *connPoolRoundRobin) Close() error {
	if !pool.closed.CompareAndSwap(false, true) {
		return ErrClientConnPoolClosed
	}

	close(pool.closeSignal)

	// wait for a running refresher tick to finish.
	pool.bmu.Lock()
	defer pool.bmu.Unlock()

	conns := make([]*pooledConn, 0, len(pool.slots))

	for _, slot := range pool.slots {
		slot.mu.Lock()
		slot.closed = true
		conns = append(conns, slot.cur)
		slot.mu.Unlock()
	}

	pool.dmu.Lock()
	for conn := range pool.draining {
		conns = append(conns, conn)
	}
	pool.dmu.Unlock()

	var wg sync.WaitGroup

	for _, conn := range conns {
		wg.Add(1)

		go func(c *pooledConn) {
			defer wg.Done()

			c.close()
		}(conn)
	}

	wg.Wait()

	return nil
}

// get returns a connection with an RPC registered on it; the caller must
// release it.
func (pool *connPoolRoundRobin) get() (*pooledConn, error) {
	if pool.closed.Load() {
		return nil, ErrClientConnPoolClosed
	}

	n := uint64(len(pool.slots))
	start := pool.next.Add(1) - 1

	for i := uint64(0); i < n; i++ {
		if conn := pool.slots[(start+i)%n].acquire(true); conn != nil {
			return conn, nil
		}
	}

	// no usable connection: replace the one of this turn and send the RPC on
	// whatever the slot holds afterwards (possibly a replacement installed
	// by a concurrent call).
	slot := pool.slots[start%n]

	if err := pool.refresh(slot, slot.current()); err != nil {
		return nil, fmt.Errorf("%w: failed to refresh connection", err)
	}

	if conn := slot.acquire(false); conn != nil {
		return conn, nil
	}

	return nil, ErrClientConnPoolClosed
}

// stale reports whether conn must be replaced.
func (pool *connPoolRoundRobin) stale(conn *pooledConn) bool {
	return !conn.usable() || conn.expired(time.Now())
}

// refresh replaces observed, the connection the caller found stale in slot,
// with a new one. It is a compare-and-set: when the slot no longer holds
// observed (a concurrent call replaced it) or observed recovered, the new
// connection is discarded. The replaced connection is closed once the RPCs
// running on it end.
func (pool *connPoolRoundRobin) refresh(slot *clientConn, observed *pooledConn) error {
	if pool.closed.Load() {
		return ErrClientConnPoolClosed
	}

	fresh, err := pool.dial()
	if err != nil {
		return err
	}

	slot.mu.Lock()

	if slot.closed {
		slot.mu.Unlock()
		fresh.close()

		return ErrClientConnPoolClosed
	}

	if slot.cur != observed || !pool.stale(observed) {
		slot.mu.Unlock()
		fresh.close()

		return nil
	}

	slot.cur = fresh

	pool.dmu.Lock()
	pool.draining[observed] = struct{}{}
	pool.dmu.Unlock()

	observed.retire(pool.opts.drainTimeout)
	slot.mu.Unlock()

	return nil
}

func (pool *connPoolRoundRobin) dial() (*pooledConn, error) {
	cc, err := pool.dialer.Dial()
	if err != nil {
		return nil, err
	}

	conn := &pooledConn{
		id:      uuid.New().String(),
		cc:      cc,
		onClose: pool.forget,
	}

	if l := pool.opts.computeConnLifetime(); l > 0 {
		conn.deadline = time.Now().Add(l)
	}

	return conn, nil
}

func (pool *connPoolRoundRobin) forget(conn *pooledConn) {
	pool.dmu.Lock()
	delete(pool.draining, conn)
	pool.dmu.Unlock()
}

func (pool *connPoolRoundRobin) refresher() {
	ticker := time.NewTicker(pool.opts.healthCheckFreq)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pool.refreshStale()

		case <-pool.closeSignal:
			return
		}
	}
}

func (pool *connPoolRoundRobin) refreshStale() {
	pool.bmu.Lock()
	defer pool.bmu.Unlock()

	if pool.closed.Load() {
		return
	}

	var wg sync.WaitGroup

	for _, slot := range pool.slots {
		conn := slot.current()
		if !pool.stale(conn) {
			continue
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := pool.refresh(slot, conn); err != nil && !errors.Is(err, ErrClientConnPoolClosed) {
				log.Printf("%s: grpc conn pool warning, failed to refresh connection %s", err, conn.id)
			}
		}()
	}

	wg.Wait()
}

// pooledStream releases its pooled connection when the stream ends, following
// the rules of grpc.ClientConn.NewStream: the context is done, RecvMsg returns
// an error, or Header/SendMsg return an error other than io.EOF.
type pooledStream struct {
	grpc.ClientStream

	once    sync.Once
	release func()
	stop    func() bool
}

func newPooledStream(ctx context.Context, stream grpc.ClientStream, release func()) *pooledStream {
	s := &pooledStream{ClientStream: stream, release: release}
	s.stop = context.AfterFunc(ctx, s.finish)

	return s
}

func (s *pooledStream) finish() {
	s.once.Do(s.release)
}

func (s *pooledStream) end() {
	s.stop()
	s.finish()
}

func (s *pooledStream) Header() (metadata.MD, error) {
	md, err := s.ClientStream.Header()
	if err != nil && !errors.Is(err, io.EOF) {
		s.end()
	}

	return md, err
}

func (s *pooledStream) SendMsg(m any) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil && !errors.Is(err, io.EOF) {
		s.end()
	}

	return err
}

func (s *pooledStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.end()
	}

	return err
}
