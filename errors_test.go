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
	errDummySentinelOne   = errors.New("dummy one")
	errDummySentinelTwo   = errors.New("dummy two")
	errDummySentinelThree = errors.New("dummy three")
	errDummySentinelFour  = errors.New("dummy four")
)

func TestNewErrorMapper(t *testing.T) {
	em := grpcx.NewErrorMapper(codes.Unknown).
		With(codes.Canceled, context.Canceled).
		With(codes.DeadlineExceeded, context.DeadlineExceeded).
		With(codes.InvalidArgument, errDummySentinelOne).
		With(codes.PermissionDenied, errDummySentinelTwo).
		With(codes.NotFound, errDummySentinelThree, errDummySentinelFour)

	require.EqualValues(t, codes.Canceled, status.Code(em.Map(context.Canceled)))
	require.EqualValues(t, codes.DeadlineExceeded, status.Code(em.Map(context.DeadlineExceeded)))
	require.EqualValues(t, codes.InvalidArgument, status.Code(em.Map(errDummySentinelOne)))
	require.EqualValues(t, codes.PermissionDenied, status.Code(em.Map(errDummySentinelTwo)))
	require.EqualValues(t, codes.NotFound, status.Code(em.Map(errDummySentinelThree)))
	require.EqualValues(t, codes.NotFound, status.Code(em.Map(errDummySentinelFour)))
}

func TestNewBaseErrorMapper(t *testing.T) {
	em := grpcx.NewBaseErrorMapper(codes.Unknown)

	require.EqualValues(t, codes.Canceled, status.Code(em.Map(context.Canceled)))
	require.EqualValues(t, codes.DeadlineExceeded, status.Code(em.Map(context.DeadlineExceeded)))
	require.EqualValues(t, codes.Unknown, status.Code(em.Map(errDummySentinelOne)))
}
