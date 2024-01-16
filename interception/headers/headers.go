package headers

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor returns a new unary client interceptor that injects
// the given list of headers into the context. Commonly used to inject
// authentication headers in outgoing requests.
func UnaryClientInterceptor(headers map[string]string) grpc.UnaryClientInterceptor {
	kvs := make([]string, 0)
	for k, v := range headers {
		kvs = append(kvs, k, v)
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, kvs...)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor similar to UnaryClientInterceptor, but for streaming
// requests.
func StreamClientInterceptor(headers map[string]string) grpc.StreamClientInterceptor {
	kvs := make([]string, 0)
	for k, v := range headers {
		kvs = append(kvs, k, v)
	}

	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx = metadata.AppendToOutgoingContext(ctx, kvs...)
		cs, err := cc.NewStream(ctx, desc, method, opts...)

		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			return nil, err
		}

		return cs, err
	}
}
