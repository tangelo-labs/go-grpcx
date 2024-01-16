package headers_test

import (
	"context"
	"testing"
	"time"

	"github.com/Avalanche-io/counter"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx/interception/headers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryClientInterceptor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stubHeaders := map[string]string{
		"foo":                     "bar",
		gofakeit.LoremIpsumWord(): gofakeit.LoremIpsumWord(),
		gofakeit.LoremIpsumWord(): gofakeit.LoremIpsumWord(),
		gofakeit.LoremIpsumWord(): gofakeit.LoremIpsumWord(),
	}

	invokesCalls := counter.NewUnsigned()
	sentMD := metadata.MD{}

	interceptor := headers.UnaryClientInterceptor(stubHeaders)
	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		invokesCalls.Add(1)

		md, _ := metadata.FromOutgoingContext(ctx)
		sentMD = md

		return nil
	}

	err := interceptor(ctx, "grpc.test.method", nil, nil, nil, invoker)

	require.NoError(t, err)
	require.EqualValues(t, 1, invokesCalls.Get())

	for k, v := range stubHeaders {
		require.Contains(t, sentMD, k)
		require.EqualValues(t, v, sentMD.Get(k)[0])
	}
}
