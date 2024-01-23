package grpcx

import (
	"sync/atomic"
)

// Balancer is a generic and thread-safe round-robin balancer.
// It is guaranteed that two concurrent calls to Balancer.Next will not return the same
// item, when the slice contains more than one item.
type Balancer[T any] struct {
	items []T
	idx   atomic.Uint64
}

// NewBalancer creates a new Balancer instance.
func NewBalancer[T any](items ...T) *Balancer[T] {
	return &Balancer[T]{
		items: items,
	}
}

// Current returns the current item in the slice, without advancing the Loadbalancer.
func (b *Balancer[T]) Current() T {
	idx := b.idx.Load()
	key := idx % uint64(len(b.items))

	return b.items[key]
}

// Next returns the next item in the slice.
// When the end of the slice is reached, it starts again from the beginning.
func (b *Balancer[T]) Next() T {
	idx := b.idx.Add(1) - 1
	key := idx % uint64(len(b.items))

	return b.items[key]
}

// Reset resets the Balancer to its initial state.
func (b *Balancer[T]) Reset() {
	b.idx.Store(0)
}
