package grpcx_test

import (
	"sync"
	"testing"

	"github.com/tangelo-labs/go-grpcx"
)

func TestBalancer_Next(t *testing.T) {
	set := []int{1, 2, 3}
	balancer := grpcx.NewBalancer[int](set...)

	for i := 0; i < 10; i++ {
		if balancer.Next() != set[i%len(set)] {
			t.Errorf("expected %d, got %d", set[i%len(set)], balancer.Next())
		}
	}
}

func TestBalancer_Next_ThreadSafe(t *testing.T) {
	var set []int

	for i := 0; i < 2000; i++ {
		set = append(set, i)
	}

	lb := grpcx.NewBalancer[int](set...)

	var wg sync.WaitGroup

	for i := 0; i < 1337; i++ {
		wg.Add(1)

		go func() {
			lb.Next()
			wg.Done()
		}()
	}

	wg.Wait()

	if lb.Next() != 1337 {
		t.Errorf("expected %d, got %d", 1337, lb.Next())
	}
}
