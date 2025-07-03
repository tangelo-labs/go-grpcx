package metadata

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor assumes that the incoming context contains metadata
// and extracts the value for the given key, appending it to the outgoing
// context. If the key is not present in the incoming metadata, it simply
// invokes the original invoker without modifying the context.
func UnaryClientInterceptor(key string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, err := contextMetadata(ctx)
		if err != nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		val := md.Get(key)
		if len(val) > 0 {
			ctx = metadata.AppendToOutgoingContext(ctx, key, val[0])
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

		ctx = metadata.AppendToOutgoingContext(ctx, key, val[0])

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// contextMetadata fetches the metadata from the given context.
func contextMetadata(ctx context.Context) (metadata.MD, error) {
	md, ok := metadata.FromIncomingContext(ctx)

	if !ok {
		return nil, errors.New("no metadata in context")
	}

	return md, nil
}
