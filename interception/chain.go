package interception

import (
	"context"

	"google.golang.org/grpc"
)

// ChainUnaryServer creates a single interceptor out of a chain of many interceptors.
// Execution is done in left-to-right order, including passing of context.
func ChainUnaryServer(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	n := len(interceptors)

	// Basic interceptor to avoid returning nil.
	if n == 0 {
		return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var (
			chain grpc.UnaryHandler
			i     = 0
		)

		chain = func(currentCtx context.Context, currentReq interface{}) (interface{}, error) {
			if i == n-1 {
				return handler(currentCtx, currentReq)
			}

			i++

			return interceptors[i](currentCtx, currentReq, info, chain)
		}

		return interceptors[0](ctx, req, info, chain)
	}
}

// ChainStreamServer creates a single interceptor out of a chain of many interceptors.
// Execution is done in left-to-right order, including passing of context.
func ChainStreamServer(interceptors ...grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	n := len(interceptors)

	// Basic interceptor to avoid returning nil.
	if n == 0 {
		return func(srv interface{}, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, stream)
		}
	}

	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		var (
			chain grpc.StreamHandler
			i     = 0
		)

		chain = func(currentSrv interface{}, currentStream grpc.ServerStream) error {
			if i == n-1 {
				return handler(currentSrv, currentStream)
			}

			i++

			return interceptors[i](currentSrv, currentStream, info, chain)
		}

		return interceptors[0](srv, stream, info, chain)
	}
}
