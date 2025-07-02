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
)

// PoolOption is a function that modifies the pool options.
type PoolOption func(*poolOptions)

// PoolDialer is responsible for creating a new connection for the pool.
type PoolDialer interface {
	Dial(context.Context) (ClientConn, error)
}

// PoolDialerFunc is a function type that implements the PoolDialer interface.
type PoolDialerFunc func(context.Context) (ClientConn, error)

// Dial implements the PoolDialer interface for PoolDialerFunc.
func (p PoolDialerFunc) Dial(ctx context.Context) (ClientConn, error) {
	return p(ctx)
}

type poolOptions struct {
	poolSize     int
	dialTimeout  time.Duration
	connLifetime time.Duration
	jitter       time.Duration
}

// computeConnLifeTime returns a randomized connection lifetime duration.
func (o *poolOptions) computeConnLifetime() time.Duration {
	if o.connLifetime <= 0 {
		return 0
	}

	rd := rand.New(rand.NewSource(time.Now().UnixNano())).Float64() + 0.5

	return time.Duration(float64(o.connLifetime) + (rd * float64(o.jitter)))
}

// WithPoolSize sets the number of connections in the pool.
func WithPoolSize(size int) PoolOption {
	return func(o *poolOptions) {
		o.poolSize = size
	}
}

// WithPoolDialTimeout sets the timeout for dialing a new connection.
func WithPoolDialTimeout(timeout time.Duration) PoolOption {
	return func(o *poolOptions) {
		o.dialTimeout = timeout
	}
}

// WithPoolConnLifetime sets the lifetime of each connection in the pool.
// Defaults to 0, which means connections will not be closed automatically.
func WithPoolConnLifetime(lifetime time.Duration) PoolOption {
	return func(o *poolOptions) {
		o.connLifetime = lifetime
	}
}

// WithPoolJitter is a random duration used to prevent from all connections
// being closed at the same time when the connection lifetime is set.
//
// This option has no effect if the connection lifetime is not set.
// Defaults to 10 seconds.
func WithPoolJitter(jitter time.Duration) PoolOption {
	return func(o *poolOptions) {
		o.jitter = jitter
	}
}

// ErrClientConnPoolClosed is returned when the connection pool is closed
// and an operation is attempted on it.
var ErrClientConnPoolClosed = errors.New("grpc conn pool is closed")

type clientConn struct {
	id        string
	cc        ClientConn
	createdAt time.Time

	deadline time.Time
	mu       sync.Mutex
}

func wrapToClientConn(cc ClientConn) *clientConn {
	return &clientConn{
		id:        uuid.New().String(),
		cc:        cc,
		createdAt: time.Now(),
	}
}

func (c *clientConn) isHealthy() bool {
	return c.cc.GetState() == connectivity.Ready || (!c.deadline.IsZero() && time.Now().Before(c.deadline))
}

func (c *clientConn) setDeadline(d time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deadline = d
}

// ClientConn is an abstraction for grpc.ClientConn.
type ClientConn interface {
	GetState() connectivity.State

	grpc.ClientConnInterface
	io.Closer
}

type connPoolRoundRobin struct {
	opts   *poolOptions
	dialer PoolDialer

	balancer *Balancer[*clientConn]
	bmu      sync.Mutex

	closed      atomic.Bool
	closeSignal chan struct{}
}

// NewClientConnPool returns a new instance of ClientConn that uses a pool of
// client connections to the backend. The pool is created with the given
// dialer and options.
//
// The pool size is determined by the WithPoolSize option.
func NewClientConnPool(dialer PoolDialer, o ...PoolOption) (ClientConn, error) {
	opts := &poolOptions{
		poolSize:     10,
		dialTimeout:  time.Minute,
		connLifetime: 0,
		jitter:       10 * time.Second,
	}

	for _, opt := range o {
		opt(opts)
	}

	pool := &connPoolRoundRobin{
		dialer:      dialer,
		opts:        opts,
		closed:      atomic.Bool{},
		closeSignal: make(chan struct{}),
	}

	conns := make(chan *clientConn, opts.poolSize)
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
		return nil, err
	}

	wraps := make([]*clientConn, 0)
	for conn := range conns {
		wraps = append(wraps, conn)
	}

	pool.balancer = NewBalancer(wraps...)

	go pool.refresher()

	return pool, nil
}

func (pool *connPoolRoundRobin) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	conn, err := pool.get()
	if err != nil {
		return fmt.Errorf("%w: failed to get a connection from the pool", err)
	}

	if iErr := conn.cc.Invoke(ctx, method, args, reply, opts...); iErr != nil {
		return iErr
	}

	return nil
}

func (pool *connPoolRoundRobin) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	conn, err := pool.get()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get a connection from the pool", err)
	}

	stream, err := conn.cc.NewStream(ctx, desc, method, opts...)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (pool *connPoolRoundRobin) GetState() connectivity.State {
	if pool.closed.Load() {
		return connectivity.Shutdown
	}

	conn := pool.balancer.Next()
	if conn == nil {
		return connectivity.Idle
	}

	return conn.cc.GetState()
}

func (pool *connPoolRoundRobin) Close() error {
	if !pool.closed.CompareAndSwap(false, true) {
		return ErrClientConnPoolClosed
	}

	pool.bmu.Lock()
	defer pool.bmu.Unlock()

	var wg sync.WaitGroup

	for i := 0; i < len(pool.balancer.items); i++ {
		wg.Add(1)

		go func(conn *clientConn) {
			defer wg.Done()

			if err := conn.cc.Close(); err != nil {
				log.Printf("%s: grpc conn pool warning, failed to close connection %s", err, conn.id)
			}
		}(pool.balancer.items[i])
	}

	wg.Wait()

	pool.balancer = nil
	close(pool.closeSignal)

	return nil
}

func (pool *connPoolRoundRobin) get() (*clientConn, error) {
	if pool.closed.Load() {
		return nil, ErrClientConnPoolClosed
	}

	conn := pool.balancer.Next()

	// if current connection is unhealthy, serve the RPC from next available healthy connection
	if !conn.isHealthy() {
		newHealthyFound := false
		startID := conn.id

		for {
			next := pool.balancer.Next()
			if startID == next.id {
				break
			}

			if next.isHealthy() {
				conn = next
				newHealthyFound = true

				break
			}
		}

		if !newHealthyFound {
			if err := pool.refresh(conn); err != nil {
				return nil, fmt.Errorf("%w: failed to refresh connection", err)
			}
		}
	}

	return conn, nil
}

func (pool *connPoolRoundRobin) refresh(conn *clientConn) error {
	ctx, cancel := context.WithTimeout(context.Background(), pool.opts.dialTimeout)
	defer cancel()

	newCC, err := pool.dialer.Dial(ctx)
	if err != nil {
		return err
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	// another goroutine might have already refreshed the connection.
	// we just drop the new connection if the old one is still healthy.
	if conn.isHealthy() {
		defer func() { _ = newCC.Close() }()

		return nil
	}

	// close old connection in a goroutine to avoid blocking
	go func(cc ClientConn) { _ = cc.Close() }(conn.cc)

	conn.cc = newCC
	conn.createdAt = time.Now()

	if l := pool.opts.computeConnLifetime(); l > 0 {
		conn.setDeadline(time.Now().Add(l))
	}

	return nil
}

func (pool *connPoolRoundRobin) dial() (*clientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pool.opts.dialTimeout)
	defer cancel()

	conn, err := pool.dialer.Dial(ctx)
	if err != nil {
		return nil, err
	}

	w := wrapToClientConn(conn)

	if l := pool.opts.computeConnLifetime(); l > 0 {
		w.setDeadline(time.Now().Add(l))
	}

	return w, nil
}

func (pool *connPoolRoundRobin) refresher() {
	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-ticker.C:
			pool.bmu.Lock()

			unhealthy := make([]*clientConn, 0)

			for i := 0; i < len(pool.balancer.items); i++ {
				if !pool.balancer.items[i].isHealthy() {
					unhealthy = append(unhealthy, pool.balancer.items[i])
				}
			}

			var wg sync.WaitGroup

			for _, conn := range unhealthy {
				wg.Add(1)

				go func(c *clientConn) {
					defer wg.Done()

					if err := pool.refresh(c); err != nil {
						log.Printf("%s: grpc conn pool warning, failed to refresh connection %s", err, c.id)
					}
				}(conn)
			}

			wg.Wait()

			pool.bmu.Unlock()

		case <-pool.closeSignal:
			ticker.Stop()

			return
		}
	}
}
