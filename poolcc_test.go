package grpcx_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

func TestNewClientConnPool_Basic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cc := &mockCC{}
	cc.
		On("GetState").Return(connectivity.Ready).
		On("Invoke", ctx, "/hello.world", nil, nil, mock.Anything).Return(nil)

	pool, err := grpcx.NewClientConnPool(
		grpcx.PoolDialerFunc(func(context.Context) (grpcx.ClientConn, error) {
			return cc, nil
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, pool)

	err = pool.Invoke(ctx, "/hello.world", nil, nil)
	require.NoError(t, err)

	cc.AssertExpectations(t)
}

func TestNewClientConnPool_Reconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cc := &mockCC{}
	cc.
		On("GetState").Once().Return(connectivity.TransientFailure).
		On("GetState").Once().Return(connectivity.Ready).
		On("Close").Maybe().Return(nil).
		On("Invoke", ctx, "/hello.world", nil, nil, mock.Anything).Return(nil)

	mpd := &mockPoolDialer{}
	mpd.On("Dial", mock.Anything).Return(cc, nil)

	pool, err := grpcx.NewClientConnPool(mpd)

	require.NoError(t, err)
	err = pool.Invoke(ctx, "/hello.world", nil, nil)
	require.NoError(t, err)

	cc.AssertExpectations(t)
	mpd.AssertExpectations(t)
}

type mockPoolDialer struct {
	mock.Mock

	grpcx.PoolDialer
}

func (m *mockPoolDialer) Dial(ctx context.Context) (grpcx.ClientConn, error) {
	args := m.Called(ctx)

	return args.Get(0).(grpcx.ClientConn), args.Error(1)
}

type mockCC struct {
	mock.Mock

	grpcx.ClientConn
}

func (f *mockCC) GetState() connectivity.State {
	arg := f.Called()

	return arg.Get(0).(connectivity.State)
}

func (f *mockCC) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	arg := f.Called(ctx, method, args, reply, opts)

	return arg.Error(0)
}

func (f *mockCC) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	arg := f.Called(ctx, desc, method, opts)

	return arg.Get(0).(grpc.ClientStream), arg.Error(1)
}

func (f *mockCC) Close() error {
	arg := f.Called()

	return arg.Error(0)
}
