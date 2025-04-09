package correlation

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const correlationIDTransportKey = "x-correlation-id"

type contextKey uint8

var correlationKey = contextKey(157)

// UnaryClientInterceptor returns a new unary client interceptor that injects the
// correlation id from the context into the outgoing metadata. If the correlation
// id is not present in the context, it will be ignored and the request will
// proceed without it.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		cID, err := fromContextMetadata(ctx)
		if err != nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}

		cCtx := context.WithValue(ctx, correlationKey, cID)

		return invoker(cCtx, method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor similar to UnaryClientInterceptor, but for streaming
// requests.
func StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		cID, err := fromContextMetadata(ctx)
		if err != nil {
			return streamer(ctx, desc, cc, method, opts...)
		}

		cCtx := context.WithValue(ctx, correlationKey, cID)
		s, err := streamer(cCtx, desc, cc, method, opts...)

		if err != nil {
			return nil, err
		}

		return s, err
	}
}

// fromContextMetadata fetches the correlation id from within the given context metadata.
func fromContextMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)

	if !ok {
		md, ok = metadata.FromOutgoingContext(ctx)

		if !ok {
			return "", errors.New("no metadata in context")
		}
	}

	key := md.Get(correlationIDTransportKey)
	if len(key) != 1 {
		return "", fmt.Errorf("correlation id key not found, include `%s` in header", correlationIDTransportKey)
	}

	return key[0], nil
}
