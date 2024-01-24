package grpcx

import (
	"context"
	"io"
	"log"

	"google.golang.org/grpc"
)

// ClientConn is an abstraction for grpc.ClientConn.
type ClientConn interface {
	grpc.ClientConnInterface
	io.Closer
}

type connPoolRoundRobin struct {
	conns    []*grpc.ClientConn
	balancer *Balancer[*grpc.ClientConn]
}

// NewClientConnPool returns a new instance of ClientConn that uses a pool of
// grpc.ClientConn instances when calling Invoke and NewStream using a round-robin
// strategy.
//
// The returned ClientConn is safe for concurrent use by multiple goroutines.
func NewClientConnPool(conns ...*grpc.ClientConn) ClientConn {
	return &connPoolRoundRobin{
		conns:    conns,
		balancer: NewBalancer[*grpc.ClientConn](conns...),
	}
}

func (cp *connPoolRoundRobin) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	err := cp.balancer.Next().Invoke(ctx, method, args, reply, opts...)

	if err != nil {
		return err
	}

	return nil
}

func (cp *connPoolRoundRobin) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	stream, err := cp.balancer.Next().NewStream(ctx, desc, method, opts...)

	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (cp *connPoolRoundRobin) Close() error {
	for i, conn := range cp.conns {
		if err := conn.Close(); err != nil {
			log.Printf("%s: grpc conn pool warning, failed to close connection %d", err, i)
		}
	}

	return nil
}
