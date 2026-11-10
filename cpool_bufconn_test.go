package grpcx_test

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// These tests run the pool against a real in-process gRPC server (bufconn),
// dialed through grpcx.Dialer exactly like production clients do
// (grpc.NewClient: connections start IDLE and go back to IDLE after the
// channel idle timeout).

// TestClientConnPool_FirstBurstOnFreshPool reproduces the staging failure:
// a burst of concurrent RPCs on a pool whose connections are all IDLE.
func TestClientConnPool_FirstBurstOnFreshPool(t *testing.T) {
	srv := startBufconnServer(t, 20*time.Millisecond)
	pool := dialBufconnPool(t, srv, "pool.size=10")

	errs := burst(t, pool, 50)

	require.Empty(t, errs, "RPCs failed on a fresh pool: %v", errs)
}

// TestClientConnPool_BurstAfterIdleTimeout warms every connection up, lets
// the channels go back to IDLE (short grpc idle timeout) and sends a burst.
func TestClientConnPool_BurstAfterIdleTimeout(t *testing.T) {
	srv := startBufconnServer(t, 20*time.Millisecond)
	pool := dialBufconnPool(t, srv, "pool.size=10", grpc.WithIdleTimeout(100*time.Millisecond))

	for i := 0; i < 10; i++ {
		require.NoError(t, check(pool))
	}

	time.Sleep(500 * time.Millisecond)

	errs := burst(t, pool, 50)

	require.Empty(t, errs, "RPCs failed after idle timeout: %v", errs)
}

// TestClientConnPool_RefresherDoesNotChurnIdleConnections checks that the
// background health check does not replace (dial + close) connections that
// are merely IDLE: on v1.0.0 every IDLE connection is redialed on each tick.
func TestClientConnPool_RefresherDoesNotChurnIdleConnections(t *testing.T) {
	srv := startBufconnServer(t, 0)

	var dials atomic.Int64

	pool, err := grpcx.NewClientConnPool(grpcx.PoolDialerFunc(func() (grpcx.ClientConn, error) {
		dials.Add(1)

		return grpc.NewClient("passthrough:///bufnet",
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return srv.lis.DialContext(ctx) }),
		)
	}), grpcx.WithPoolSize(4), grpcx.WithPoolHealthCheckFreq(10*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })

	time.Sleep(200 * time.Millisecond)

	require.EqualValues(t, 4, dials.Load(), "IDLE connections were replaced by the health check")
	require.NoError(t, check(pool))
}

// TestClientConnPool_ConcurrentRPCsDuringRefreshNoRace is meant to run with
// -race: RPCs read the pooled *grpc.ClientConn while the pool replaces it.
func TestClientConnPool_ConcurrentRPCsDuringRefreshNoRace(t *testing.T) {
	srv := startBufconnServer(t, time.Millisecond)
	pool := dialBufconnPool(t, srv, "pool.size=2&pool.healthCheckFreq=1ms", grpc.WithIdleTimeout(10*time.Millisecond))

	deadline := time.Now().Add(300 * time.Millisecond)

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for time.Now().Before(deadline) {
				_ = check(pool)
			}
		}()
	}

	wg.Wait()
}

// TestClientConnPool_RecoversAfterServerRestart runs the real gRPC connection
// states (READY → IDLE/TRANSIENT_FAILURE → READY) through the pool: while
// the server is down RPCs fail as Unavailable, never cancelled by the pool,
// and once it is back a burst succeeds.
func TestClientConnPool_RecoversAfterServerRestart(t *testing.T) {
	var lis atomic.Pointer[bufconn.Listener]

	start := func() *grpc.Server {
		l := bufconn.Listen(1 << 20)
		lis.Store(l)

		gs := grpc.NewServer()
		healthpb.RegisterHealthServer(gs, health.NewServer())

		go func() { _ = gs.Serve(l) }()

		t.Cleanup(gs.Stop)

		return gs
	}

	gs := start()

	cfg, err := grpcx.ParseClientConfig("grpc://bufnet:1?tls=false&resolver.scheme=passthrough&pool.size=4&pool.healthCheckFreq=10ms")
	require.NoError(t, err)

	pool, err := cfg.NewDialer().WithOptions(
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.Load().DialContext(ctx) }),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff:           backoff.Config{BaseDelay: 10 * time.Millisecond, Multiplier: 1, MaxDelay: 10 * time.Millisecond},
			MinConnectTimeout: 100 * time.Millisecond,
		}),
	).Dial()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })

	require.Empty(t, burst(t, pool, 20))

	gs.Stop()

	var failed int

	for deadline := time.Now().Add(200 * time.Millisecond); time.Now().Before(deadline); {
		if err := check(pool); err != nil {
			failed++

			require.Equal(t, codes.Unavailable, status.Code(err), "RPC failed during the outage with: %v", err)
		}
	}

	require.Positive(t, failed, "the outage was not observed")

	start()

	require.Eventually(t, func() bool { return check(pool) == nil }, 2*time.Second, 10*time.Millisecond)
	require.Empty(t, burst(t, pool, 50), "RPCs failed after the server came back")
}

type bufconnServer struct {
	lis   *bufconn.Listener
	calls atomic.Int64
}

func startBufconnServer(t *testing.T, delay time.Duration) *bufconnServer {
	t.Helper()

	s := &bufconnServer{lis: bufconn.Listen(1 << 20)}
	gs := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		s.calls.Add(1)
		time.Sleep(delay)

		return h(ctx, req)
	}))
	healthpb.RegisterHealthServer(gs, health.NewServer())

	go func() { _ = gs.Serve(s.lis) }()

	t.Cleanup(gs.Stop)

	return s
}

func dialBufconnPool(t *testing.T, srv *bufconnServer, params string, opts ...grpc.DialOption) grpcx.ClientConn {
	t.Helper()

	cfg, err := grpcx.ParseClientConfig("grpc://bufnet:1?tls=false&resolver.scheme=passthrough&" + params)
	require.NoError(t, err)

	opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		// emulate TCP + TLS handshake latency so channels spend some time CONNECTING.
		time.Sleep(5 * time.Millisecond)

		return srv.lis.DialContext(ctx)
	}))

	pool, err := cfg.NewDialer().WithOptions(opts...).Dial()
	require.NoError(t, err)

	t.Cleanup(func() { _ = pool.Close() })

	return pool
}

func check(cc grpc.ClientConnInterface) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := healthpb.NewHealthClient(cc).Check(ctx, &healthpb.HealthCheckRequest{})

	return err
}

func burst(t *testing.T, cc grpc.ClientConnInterface, n int) []error {
	t.Helper()

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			if err := check(cc); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}

	close(start)
	wg.Wait()

	return errs
}
