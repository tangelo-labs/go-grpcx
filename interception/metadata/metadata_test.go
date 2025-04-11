package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryClientInterceptor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := "x-correlation-id"
	value := "test-correlation-id"

	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(key, value))
	interceptor := UnaryClientInterceptor(key)

	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)

		cID := md.Get(key)
		require.Len(t, cID, 1)
		require.Equal(t, value, cID[0])

		return nil
	}

	err := interceptor(ctx, "test.method", nil, nil, nil, invoker)
	require.NoError(t, err)
}

func TestStreamClientInterceptor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	key := "x-correlation-id"
	value := "test-correlation-id"

	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(key, value))
	interceptor := StreamClientInterceptor(key)

	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)

		cID := md.Get(key)
		require.Len(t, cID, 1)
		require.Equal(t, value, cID[0])

		return nil, nil
	}

	_, err := interceptor(ctx, &grpc.StreamDesc{}, nil, "test.method", streamer)
	require.NoError(t, err)
}
