package semaphore_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/CrimsonCoder42/toolbench/semaphore"
)

func ExampleWeighted() {
	ctx := context.Background()
	s := semaphore.New(3)

	fmt.Println(s.Acquire(ctx, 2)) // 1 permit left
	fmt.Println(s.TryAcquire(2))   // does not fit
	fmt.Println(s.TryAcquire(1))   // fits: 0 left
	s.Release(3)
	fmt.Println(s.TryAcquire(3))
	// Output:
	// <nil>
	// false
	// true
	// true
}

func ExampleNew() {
	s := semaphore.New(-1) // a negative size is size 0, not a panic
	fmt.Println(s.TryAcquire(0), s.TryAcquire(1))
	// Output: true false
}

func ExampleErrTooLarge() {
	s := semaphore.New(3)
	err := s.Acquire(context.Background(), 4) // returns at once; x/sync would block until ctx is done
	fmt.Println(errors.Is(err, semaphore.ErrTooLarge))
	fmt.Println(err)
	// Output:
	// true
	// semaphore: weight exceeds size: n=4 size=3
}

func ExampleErrNegativeWeight() {
	s := semaphore.New(3)
	err := s.Acquire(context.Background(), -1)
	fmt.Println(errors.Is(err, semaphore.ErrNegativeWeight))
	// Output: true
}
