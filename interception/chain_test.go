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

var (
	testService      = "SomeService.StreamMethod"
	parentUnaryInfo  = &grpc.UnaryServerInfo{FullMethod: testService}
	parentStreamInfo = &grpc.StreamServerInfo{FullMethod: testService}
	testValue        = 1
	ctx              = context.WithValue(context.Background(), ctxKey("parent"), testValue)
)

type ctxKey string

func TestChainUnaryServer(t *testing.T) {
	input := "input"
	output := "output"

	first := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requireContextValue(ctx, t, ctxKey("parent"))
		require.Equal(t, parentUnaryInfo, info)
		ctx = context.WithValue(ctx, ctxKey("first"), 1)
		return handler(ctx, req)
	}

	second := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requireContextValue(ctx, t, ctxKey("parent"))
		requireContextValue(ctx, t, ctxKey("first"))
		require.Equal(t, parentUnaryInfo, info)
		ctx = context.WithValue(ctx, ctxKey("second"), 1)
		return handler(ctx, req)
	}

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		require.EqualValues(t, input, req)
		requireContextValue(ctx, t, ctxKey("parent"))
		requireContextValue(ctx, t, ctxKey("first"))
		requireContextValue(ctx, t, ctxKey("second"))
		return output, nil
	}

	chain := interception.ChainServerUnary(first, second)
	out, err := chain(ctx, input, parentUnaryInfo, handler)
	require.EqualValues(t, output, out)
	require.NoError(t, err)
}

func TestChainStreamServerServer(t *testing.T) {
	t.Run("it should do nothing when no interceptors provided", func(t *testing.T) {
		require.NoError(t, interception.ChainServerStream()(nil, &fakeServerStream{}, nil, func(svc interface{}, stream grpc.ServerStream) error { return nil }))
	})

	t.Run("it should chain interceptors", func(t *testing.T) {
		first := func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx = context.WithValue(ctx, ctxKey("first"), 1)
			stream = grpcx.ServerStreamWithContext(ctx, stream)

			return handler(srv, stream)
		}

		second := func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			ctx = context.WithValue(ctx, ctxKey("second"), 1)
			stream = grpcx.ServerStreamWithContext(ctx, stream)

			return handler(srv, stream)
		}

		handler := func(svc interface{}, stream grpc.ServerStream) error {
			requireContextValue(ctx, t, ctxKey("parent"))
			requireContextValue(ctx, t, ctxKey("first"))
			requireContextValue(ctx, t, ctxKey("second"))

			return nil
		}

		chain := interception.ChainServerStream(first, second)
		stream := grpcx.ServerStreamWithContext(ctx, &fakeServerStream{ctx: ctx})

		err := chain(nil, stream, parentStreamInfo, handler)
		require.NoError(t, err)
	})
}

func requireContextValue(ctx context.Context, t *testing.T, key ctxKey, msg ...interface{}) {
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
