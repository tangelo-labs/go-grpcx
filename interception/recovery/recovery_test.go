package recovery_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Avalanche-io/counter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx/interception/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type (
	emptyServiceServer interface{}
	testServer         struct{}
)

func TestUnaryServerInterceptor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	recoveryFunc := func(ctx context.Context, p interface{}) error {
		assert.NotNil(t, p, "we was expecting a panic, so recover information cannot be nil")

		return fmt.Errorf("%v", p)
	}

	port := freeTCPAddr()
	l, err := net.Listen("tcp", port.String())
	require.NoError(t, err)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			recovery.UnaryServerInterceptor(recoveryFunc),
		),
	)

	endpointCalls := counter.NewUnsigned()
	testSd := grpc.ServiceDesc{
		ServiceName: "grpc.testing.PanicService",
		HandlerType: (*emptyServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "PanicCall",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					return interceptor(ctx, nil, nil, func(ctx context.Context, req interface{}) (interface{}, error) {
						endpointCalls.Up()

						panic("panic")
					})
				},
			},
		},
	}

	server.RegisterService(&testSd, &testServer{})

	// Starting the server
	go func() {
		err = server.Serve(l)
		require.NoError(t, err)
	}()

	cc, err := grpc.DialContext(ctx, port.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	require.NotPanics(t, func() {
		inErr := cc.Invoke(ctx, "/grpc.testing.PanicService/PanicCall", nil, nil)
		require.Error(t, inErr)
	})

	require.EqualValues(t, 1, endpointCalls.Get())
}

func freeTCPAddr() *net.TCPAddr {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}

	if err := l.Close(); err != nil {
		panic(err)
	}

	return l.Addr().(*net.TCPAddr)
}
