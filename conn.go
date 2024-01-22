package grpcx

import (
	"context"
	"io"
	"math/rand"
	"time"

	"google.golang.org/grpc"
)

// ClientConn is an abstraction for grpc.ClientConn.
type ClientConn interface {
	grpc.ClientConnInterface
	io.Closer
}

type connPool struct {
	conns []*grpc.ClientConn
}

// NewClientConnPool returns a new instance of ClientConn that uses a pool of
// grpc.ClientConn instances when calling Invoke and NewStream.
func NewClientConnPool(conns ...*grpc.ClientConn) ClientConn {
	return &connPool{
		conns: conns,
	}
}

func (cp *connPool) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	err := cp.conns[rnd.Intn(len(cp.conns))].Invoke(ctx, method, args, reply, opts...)

	if err != nil {
		return err
	}

	return nil
}

func (cp *connPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	stream, err := cp.conns[rnd.Intn(len(cp.conns))].NewStream(ctx, desc, method, opts...)

	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (cp *connPool) Close() error {
	for _, conn := range cp.conns {
		_ = conn.Close()
	}

	return nil
}
