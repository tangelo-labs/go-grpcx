package causation

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

	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(causationIDTransportKey, "test-causation-id"))
	interceptor := UnaryClientInterceptor()

	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		cID, ok := ctx.Value(causationKey).(string)
		require.True(t, ok)
		require.Equal(t, "test-causation-id", cID)

		return nil
	}

	err := interceptor(ctx, "test.method", nil, nil, nil, invoker)
	require.NoError(t, err)
}

func TestStreamClientInterceptor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(causationIDTransportKey, "test-causation-id"))
	interceptor := StreamClientInterceptor()

	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		cID, ok := ctx.Value(causationKey).(string)
		require.True(t, ok)
		require.Equal(t, "test-causation-id", cID)

		return nil, nil
	}

	_, err := interceptor(ctx, nil, nil, "test.method", streamer)
	require.NoError(t, err)
}

func TestFromContextMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("valid metadata", func(t *testing.T) {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(causationIDTransportKey, "test-causation-id"))
		cID, err := fromContextMetadata(ctx)
		require.NoError(t, err)
		require.Equal(t, "test-causation-id", cID)
	})

	t.Run("missing metadata", func(t *testing.T) {
		ctx = context.Background()
		_, err := fromContextMetadata(ctx)
		require.Error(t, err)
	})

	t.Run("invalid metadata", func(t *testing.T) {
		ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			causationIDTransportKey, "id-one",
			causationIDTransportKey, "id-two",
		))
		_, err := fromContextMetadata(ctx)
		require.Error(t, err)
	})
}
