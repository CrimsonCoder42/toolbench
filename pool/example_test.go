package pool_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/CrimsonCoder42/toolbench/pool"
)

func ExamplePool() {
	var sum atomic.Int64
	p, err := pool.New(context.Background(), 2, func(_ context.Context, v int64) error {
		sum.Add(v)
		return nil
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for v := range int64(10) {
		if err := p.Submit(context.Background(), v+1); err != nil {
			fmt.Println("submit:", err)
		}
	}
	fmt.Println(p.Close(), sum.Load())
	// Output: <nil> 55
}

func ExampleNew() {
	p, err := pool.New(context.Background(), 4, func(_ context.Context, s string) error { return nil })
	fmt.Println(err, p.Close())
	// Output: <nil> <nil>
}

func ExamplePool_Submit() {
	// The queue holds one value; a second Submit would block until a worker is free.
	release := make(chan struct{})
	p, _ := pool.New(context.Background(), 1, func(_ context.Context, _ int) error {
		<-release
		return nil
	}, pool.WithQueue(1))
	fmt.Println(p.Submit(context.Background(), 1)) // a worker or the queue takes it
	close(release)
	fmt.Println(p.Close())
	fmt.Println(errors.Is(p.Submit(context.Background(), 2), pool.ErrClosed))
	// Output:
	// <nil>
	// <nil>
	// true
}

func ExamplePool_Close() {
	errOdd := errors.New("odd")
	p, _ := pool.New(context.Background(), 2, func(_ context.Context, v int) error {
		if v%2 == 1 {
			return fmt.Errorf("%d: %w", v, errOdd)
		}
		return nil
	})
	for v := range 4 {
		_ = p.Submit(context.Background(), v)
	}
	err := p.Close() // every error, joined
	fmt.Println(errors.Is(err, errOdd), len(err.(interface{ Unwrap() []error }).Unwrap()))
	// Output: true 2
}

func ExampleOption() {
	opts := []pool.Option{pool.WithQueue(8), pool.WithFailFast()}
	p, err := pool.New(context.Background(), 2, func(context.Context, int) error { return nil }, opts...)
	fmt.Println(err, p.Close())
	// Output: <nil> <nil>
}

func ExampleWithQueue() {
	var ran atomic.Int64
	p, _ := pool.New(context.Background(), 1, func(context.Context, int) error {
		ran.Add(1)
		return nil
	}, pool.WithQueue(16))
	for v := range 16 {
		_ = p.Submit(context.Background(), v)
	}
	fmt.Println(p.Close(), ran.Load())
	// Output: <nil> 16
}

func ExampleWithFailFast() {
	errBad := errors.New("bad value")
	p, _ := pool.New(context.Background(), 1, func(_ context.Context, v int) error {
		if v == 3 {
			return errBad
		}
		return nil
	}, pool.WithFailFast())
	rejected := 0
	for v := 1; v <= 5; v++ {
		if p.Submit(context.Background(), v) != nil {
			rejected++ // 4 and 5: the only worker stopped at 3
		}
	}
	fmt.Println(p.Close(), rejected)
	// Output: bad value 2
}

func ExampleErrClosed() {
	p, _ := pool.New(context.Background(), 1, func(context.Context, int) error { return nil })
	_ = p.Close()
	fmt.Println(errors.Is(p.Submit(context.Background(), 1), pool.ErrClosed))
	// Output: true
}

func ExampleErrWorkers() {
	_, err := pool.New(context.Background(), 0, func(context.Context, int) error { return nil })
	fmt.Println(errors.Is(err, pool.ErrWorkers), err)
	// Output: true pool: worker count must be positive: got 0
}

func ExampleErrQueueFull() {
	// No queue and the only worker is inside fn: a Submit from that worker cannot be taken, so it returns at
	// once with ErrQueueFull instead of waiting for itself.
	got := make(chan error, 1)
	var p *pool.Pool[int]
	p, _ = pool.New(context.Background(), 1, func(ctx context.Context, v int) error {
		if v == 1 {
			got <- p.Submit(ctx, 2) // ctx is the worker's: non-blocking
		}
		return nil
	})
	_ = p.Submit(context.Background(), 1)
	err := <-got
	fmt.Println(errors.Is(err, pool.ErrQueueFull), err)
	fmt.Println(p.Close())
	// Output:
	// true pool: queue full: queue size 0
	// <nil>
}

func ExamplePool_TrySubmit() {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	p, _ := pool.New(context.Background(), 1, func(context.Context, int) error {
		entered <- struct{}{}
		<-release
		return nil
	}, pool.WithQueue(1))
	_ = p.Submit(context.Background(), 1) // blocking: returns once the worker or the queue has it
	<-entered                             // the worker holds 1, so the queue is empty
	ok2, _ := p.TrySubmit(2)              // the queue takes it
	ok3, err := p.TrySubmit(3)            // worker busy, queue full: refused at once
	close(release)
	fmt.Println(ok2, ok3, errors.Is(err, pool.ErrQueueFull))
	fmt.Println(p.Close())
	_, err = p.TrySubmit(4)
	fmt.Println(errors.Is(err, pool.ErrClosed))
	// Output:
	// true false true
	// <nil>
	// true
}

func ExampleWithOnDone() {
	var totalAtEnd atomic.Int64
	p, _ := pool.New(context.Background(), 2, func(context.Context, int) error { return nil },
		pool.WithOnDone(func(done, total int) {
			if done == 5 { // runs on a worker; use the arguments, never a shared counter
				totalAtEnd.Store(int64(total))
			}
		}))
	for v := range 5 {
		_ = p.Submit(context.Background(), v)
	}
	fmt.Println(p.Close(), totalAtEnd.Load())
	// Output: <nil> 5
}

func ExampleWithMaxErrors() {
	errBad := errors.New("bad")
	p, _ := pool.New(context.Background(), 1, func(_ context.Context, v int) error {
		return fmt.Errorf("value %d: %w", v, errBad)
	}, pool.WithMaxErrors(2))
	for v := range 5 {
		_ = p.Submit(context.Background(), v)
	}
	err := p.Close()
	fmt.Println(len(err.(interface{ Unwrap() []error }).Unwrap()), errors.Is(err, errBad))
	fmt.Println(err)
	// Output:
	// 3 true
	// value 0: bad
	// value 1: bad
	// pool: 3 further errors not kept: value 4: bad
}
