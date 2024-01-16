package grpcx

import (
	"context"
	"errors"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorMapper is a utility to map Go errors to gRPC status codes.
//
// This implementation is safe for concurrent use, and can be shared across
// multiple goroutines.
type ErrorMapper struct {
	m  map[error]codes.Code
	df codes.Code
	mu sync.RWMutex
}

// NewErrorMapper creates a new ErrorMapper with the given default code.
//
// The provided code will be used as default status code when no mapping is
// found for a given error.
func NewErrorMapper(defaultCode codes.Code) *ErrorMapper {
	return &ErrorMapper{
		m:  make(map[error]codes.Code),
		df: defaultCode,
	}
}

// NewBaseErrorMapper same as NewErrorMapper but creates a new instances that
// has two pre-registered rules:
//
//   - context.Canceled to codes.Canceled
//   - context.DeadlineExceeded to codes.DeadlineExceeded
func NewBaseErrorMapper(defaultCode codes.Code) *ErrorMapper {
	return NewErrorMapper(defaultCode).
		With(codes.Canceled, context.Canceled).
		With(codes.DeadlineExceeded, context.DeadlineExceeded)
}

// With registers the given list of errors to the specified status code. If an
// error is already registered, it will be overwritten.
func (em *ErrorMapper) With(code codes.Code, err ...error) *ErrorMapper {
	em.mu.Lock()
	defer em.mu.Unlock()

	for i := range err {
		em.m[err[i]] = code
	}

	return em
}

// Map maps the given error to its corresponding status code based on set of
// rules previously registered using the With method, if no mapping is found
// the default status code is used.
func (em *ErrorMapper) Map(err error) error {
	if err == nil {
		return nil
	}

	em.mu.RLock()
	defer em.mu.RUnlock()

	for e, c := range em.m {
		if errors.Is(err, e) {
			return status.Error(c, err.Error())
		}
	}

	return status.Error(em.df, err.Error())
}
