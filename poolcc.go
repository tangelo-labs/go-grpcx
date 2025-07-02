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

	"github.com/hashicorp/go-multierror"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type PoolOption func(*poolOptions)

type poolOptions struct {
	poolSize     int
	dialTimeout  time.Duration
	connLifetime time.Duration
	jitter       time.Duration
}

// connLifeTimeout returns a randomized connection lifetime duration.
func (o *poolOptions) connLifeTimeout() time.Duration {
	rd := rand.New(rand.NewSource(time.Now().UnixNano())).Float64() + 0.5

	return time.Duration(float64(o.connLifetime) + (rd * float64(o.jitter)))
}

func WithPoolSize(size int) PoolOption {
	return func(o *poolOptions) {
		o.poolSize = size
	}
}

func WithPoolDialTimeout(timeout time.Duration) PoolOption {
	return func(o *poolOptions) {
		o.dialTimeout = timeout
	}
}

func WithPoolConnLifetime(lifetime time.Duration) PoolOption {
	return func(o *poolOptions) {
		o.connLifetime = lifetime
	}
}

func WithPoolJitter(jitter time.Duration) PoolOption {
	return func(o *poolOptions) {
		o.jitter = jitter
	}
}

// ErrConnPoolClosed is returned when the connection pool is closed
// and an operation is attempted on it.
var ErrConnPoolClosed = errors.New("grpc conn pool is closed")

type clientConn struct {
	cc        *grpc.ClientConn
	createdAt time.Time

	deadline time.Time
	mu       sync.Mutex
}

func wrapToClientConn(cc *grpc.ClientConn) *clientConn {
	return &clientConn{
		cc:        cc,
		createdAt: time.Now(),
	}
}

func (c *clientConn) isHealthy() bool {
	return c.cc.GetState() == connectivity.Ready && time.Now().Before(c.deadline)
}

func (c *clientConn) setDeadline(d time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deadline = d
}

// ClientConn is an abstraction for grpc.ClientConn.
type ClientConn interface {
	grpc.ClientConnInterface
	io.Closer
}

type connPoolRoundRobin struct {
	opts     *poolOptions
	dialer   *Dialer
	balancer *Balancer[*clientConn]
	closed   atomic.Bool
}

// NewClientConnPool returns a new instance of ClientConn that uses a pool of
// client connections to the backend. The pool is created with the given
// dialer and options.
//
// The pool size is determined by the WithPoolSize option.
func NewClientConnPool(dialer *Dialer, o ...PoolOption) (ClientConn, error) {
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
		dialer: dialer,
		opts:   opts,
		closed: atomic.Bool{},
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

	return pool, nil
}

func (cp *connPoolRoundRobin) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	conn, err := cp.get()
	if err != nil {
		return fmt.Errorf("%w: failed to get a connection", err)
	}

	if iErr := conn.cc.Invoke(ctx, method, args, reply, opts...); iErr != nil {
		return iErr
	}

	return nil
}

func (cp *connPoolRoundRobin) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	conn, err := cp.get()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get a connection", err)
	}

	stream, err := conn.cc.NewStream(ctx, desc, method, opts...)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (cp *connPoolRoundRobin) Close() error {
	if !cp.closed.CompareAndSwap(false, true) {
		return ErrConnPoolClosed
	}

	items := cp.balancer.Slice()

	for i, conn := range items {
		if err := conn.cc.Close(); err != nil {
			log.Printf("%s: grpc conn pool warning, failed to close connection %d", err, i)
		}
	}

	return nil
}

func (cp *connPoolRoundRobin) get() (*clientConn, error) {
	conn := cp.balancer.Next()

	// if current connection is unhealthy, serve the RPC from next available healthy connection
	if !conn.isHealthy() {
		found := false
		items := cp.balancer.Slice()

		for i := 0; i < len(items); i++ {
			if items[i].isHealthy() {
				conn = items[i]
				found = true

				break
			}
		}

		if !found {
			if err := cp.refresh(conn); err != nil {
				return nil, fmt.Errorf("%w: failed to refresh connection", err)
			}
		}
	}

	return conn, nil
}

func (cp *connPoolRoundRobin) refresh(conn *clientConn) error {
	ctx, cancel := context.WithTimeout(context.Background(), cp.opts.dialTimeout)
	defer cancel()

	newCC, err := cp.dialer.Dial(ctx)
	if err != nil {
		return err
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	// close old connection in a goroutine to avoid blocking
	go func(cc *grpc.ClientConn) { _ = cc.Close() }(conn.cc)

	conn.cc = newCC
	conn.createdAt = time.Now()
	conn.setDeadline(time.Now().Add(cp.opts.connLifeTimeout()))

	return nil
}

func (cp *connPoolRoundRobin) dial() (*clientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cp.opts.dialTimeout)
	defer cancel()

	conn, err := cp.dialer.Dial(ctx)
	if err != nil {
		return nil, err
	}

	w := wrapToClientConn(conn)
	w.setDeadline(time.Now().Add(cp.opts.connLifeTimeout()))

	return w, nil
}
