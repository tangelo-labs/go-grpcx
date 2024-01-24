package interception_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx"
	"github.com/tangelo-labs/go-grpcx/interception"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ctxKey struct{ v string }

func TestChainUnaryServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		testService     = "SomeService.StreamMethod"
		parentUnaryInfo = &grpc.UnaryServerInfo{FullMethod: testService}
		testValue       = 1
	)

	ctx = context.WithValue(ctx, ctxKey{v: "parent"}, testValue)
	input := "input"
	output := "output"

	first := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requireContextValue(ctx, t, ctxKey{v: "parent"}, testValue)
		require.Equal(t, parentUnaryInfo, info)

		ctx = context.WithValue(ctx, ctxKey{v: "first"}, 1)

		return handler(ctx, req)
	}

	second := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requireContextValue(ctx, t, ctxKey{v: "parent"}, testValue)
		requireContextValue(ctx, t, ctxKey{v: "first"}, testValue)
		require.Equal(t, parentUnaryInfo, info)

		ctx = context.WithValue(ctx, ctxKey{v: "second"}, 1)

		return handler(ctx, req)
	}

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		require.EqualValues(t, input, req)
		requireContextValue(ctx, t, ctxKey{v: "parent"}, testValue)
		requireContextValue(ctx, t, ctxKey{v: "first"}, testValue)
		requireContextValue(ctx, t, ctxKey{v: "second"}, testValue)

		return output, nil
	}

	chain := interception.ChainServerUnary(first, second)
	out, err := chain(ctx, input, parentUnaryInfo, handler)

	require.EqualValues(t, output, out)
	require.NoError(t, err)
}

func TestChainStreamServerServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		testService      = "SomeService.StreamMethod"
		parentStreamInfo = &grpc.StreamServerInfo{FullMethod: testService}
		testValue        = 1
	)

	ctx = context.WithValue(ctx, ctxKey{v: "parent"}, testValue)

	t.Run("it should do nothing when no interceptors provided", func(t *testing.T) {
		require.NoError(t, interception.ChainServerStream()(nil, &fakeServerStream{}, nil, func(svc interface{}, stream grpc.ServerStream) error { return nil }))
	})

	t.Run("it should chain interceptors", func(t *testing.T) {
		first := func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx = context.WithValue(ctx, ctxKey{v: "first"}, 1)
			stream = grpcx.ServerStreamWithContext(ctx, stream)

			return handler(srv, stream)
		}

		second := func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx = context.WithValue(ctx, ctxKey{v: "second"}, 1)
			stream = grpcx.ServerStreamWithContext(ctx, stream)

			return handler(srv, stream)
		}

		handler := func(svc interface{}, stream grpc.ServerStream) error {
			requireContextValue(ctx, t, ctxKey{v: "parent"}, testValue)
			requireContextValue(ctx, t, ctxKey{v: "first"}, testValue)
			requireContextValue(ctx, t, ctxKey{v: "second"}, testValue)

			return nil
		}

		chain := interception.ChainServerStream(first, second)
		stream := grpcx.ServerStreamWithContext(ctx, &fakeServerStream{ctx: ctx})

		err := chain(nil, stream, parentStreamInfo, handler)
		require.NoError(t, err)
	})
}

func requireContextValue(ctx context.Context, t *testing.T, key ctxKey, testValue int, msg ...interface{}) {
	val := ctx.Value(key)

	require.NotNil(t, val, msg...)
	require.Equal(t, testValue, val, msg...)
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx         context.Context
	recvMessage interface{}
	sentMessage interface{}
}

func (f *fakeServerStream) Context() context.Context {
	return f.ctx
}

func (f *fakeServerStream) SendMsg(m interface{}) error {
	if f.sentMessage != nil {
		return status.Errorf(codes.AlreadyExists, "fakeServerStream only takes one message, sorry")
	}

	f.sentMessage = m

	return nil
}

func (f *fakeServerStream) RecvMsg(m interface{}) error {
	if f.recvMessage == nil {
		return status.Errorf(codes.NotFound, "fakeServerStream has no message, sorry")
	}

	return nil
}
