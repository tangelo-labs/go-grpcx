package recovery

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HandlerFunc is a function that recovers from the panic `p` by returning an
// `error`. The context can be used to extract request scoped metadata and
// context values.
type HandlerFunc func(ctx context.Context, p interface{}) (err error)

// UnaryServerInterceptor returns a new unary server interceptor for panic recovery.
func UnaryServerInterceptor(recovery HandlerFunc) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (_ interface{}, err error) {
		panicked := true

		defer func() {
			if r := recover(); r != nil || panicked {
				err = recoverFrom(ctx, r, recovery)
			}
		}()

		resp, err := handler(ctx, req)
		panicked = false

		return resp, err
	}
}

// StreamServerInterceptor returns a new streaming server interceptor for panic
// recovery.
func StreamServerInterceptor(recovery HandlerFunc) grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		panicked := true

		defer func() {
			if r := recover(); r != nil || panicked {
				err = recoverFrom(stream.Context(), r, recovery)
			}
		}()

		err = handler(srv, stream)
		panicked = false

		return err
	}
}

func recoverFrom(ctx context.Context, recover interface{}, handlerFunc HandlerFunc) error {
	if handlerFunc == nil {
		return status.Errorf(codes.Internal, "%v", recover)
	}

	return handlerFunc(ctx, recover)
}
