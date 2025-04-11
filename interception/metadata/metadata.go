package metadata

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type contextKey string

// UnaryClientInterceptor returns a new unary client interceptor that injects the
// correlation id from the context into the outgoing metadata. If the correlation
// id is not present in the context, it will be ignored and the request will
// proceed without it.
func UnaryClientInterceptor(key string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, err := contextMetadata(ctx)
		if err != nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		val := md.Get(key)
		if len(val) > 0 {
			ctx = context.WithValue(ctx, contextKey(key), val[0])
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor similar to UnaryClientInterceptor, but for streaming
// requests.
func StreamClientInterceptor(key string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		md, err := contextMetadata(ctx)
		if err != nil {
			return streamer(ctx, desc, cc, method, opts...)
		}

		val := md.Get(key)
		if len(val) != 1 {
			return streamer(ctx, desc, cc, method, opts...)
		}

		ctx = context.WithValue(ctx, contextKey(key), val[0])

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// contextMetadata fetches the metadata from the given context.
func contextMetadata(ctx context.Context) (metadata.MD, error) {
	md, ok := metadata.FromIncomingContext(ctx)

	if !ok {
		md, ok = metadata.FromOutgoingContext(ctx)
		if !ok {
			return nil, errors.New("no metadata in context")
		}
	}

	return md, nil
}
