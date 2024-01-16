package grpcx_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tangelo-labs/go-grpcx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	dummySentinelOne   = errors.New("dummy one")
	dummySentinelTwo   = errors.New("dummy two")
	dummySentinelThree = errors.New("dummy three")
	dummySentinelFour  = errors.New("dummy four")
)

func TestNewErrorMapper(t *testing.T) {
	em := grpcx.NewErrorMapper(codes.Unknown).
		With(codes.Canceled, context.Canceled).
		With(codes.DeadlineExceeded, context.DeadlineExceeded).
		With(codes.InvalidArgument, dummySentinelOne).
		With(codes.PermissionDenied, dummySentinelTwo).
		With(codes.NotFound, dummySentinelThree, dummySentinelFour)

	require.EqualValues(t, codes.Canceled, status.Code(em.Map(context.Canceled)))
	require.EqualValues(t, codes.DeadlineExceeded, status.Code(em.Map(context.DeadlineExceeded)))
	require.EqualValues(t, codes.InvalidArgument, status.Code(em.Map(dummySentinelOne)))
	require.EqualValues(t, codes.PermissionDenied, status.Code(em.Map(dummySentinelTwo)))
	require.EqualValues(t, codes.NotFound, status.Code(em.Map(dummySentinelThree)))
	require.EqualValues(t, codes.NotFound, status.Code(em.Map(dummySentinelFour)))
}

func TestNewBaseErrorMapper(t *testing.T) {
	em := grpcx.NewBaseErrorMapper(codes.Unknown)

	require.EqualValues(t, codes.Canceled, status.Code(em.Map(context.Canceled)))
	require.EqualValues(t, codes.DeadlineExceeded, status.Code(em.Map(context.DeadlineExceeded)))
	require.EqualValues(t, codes.Unknown, status.Code(em.Map(dummySentinelOne)))
}
