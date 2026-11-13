package breaker_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/breaker"
	"github.com/CrimsonCoder42/toolbench/clock"
)

func ExampleBreaker() {
	var b breaker.Breaker // the zero value: 5 consecutive failures open it for 60 s
	down := errors.New("connection refused")
	for range 5 {
		_ = b.Do(context.Background(), func(context.Context) error { return down })
	}
	fmt.Println(b.State())
	fmt.Println(b.Do(context.Background(), func(context.Context) error { return nil }))
	// Output:
	// open
	// breaker: circuit open
}

func ExampleNew() {
	f := clock.NewFake(start)
	b := breaker.New(
		breaker.WithFailureThreshold(2),
		breaker.WithOpenFor(30*time.Second),
		breaker.WithClock(f),
	)
	call := func(err error) {
		got := b.Do(context.Background(), func(context.Context) error { return err })
		fmt.Printf("%-9v -> %-9v %v\n", err, b.State(), got)
	}
	down := errors.New("down")
	call(down)
	call(down)
	call(nil)
	f.Advance(30 * time.Second)
	call(nil)
	// Output:
	// down      -> closed    down
	// down      -> open      down
	// <nil>     -> open      breaker: circuit open
	// <nil>     -> closed    <nil>
}

func ExampleBreaker_Do() {
	b := breaker.New(breaker.WithFailureThreshold(1))
	err := b.Do(context.Background(), func(context.Context) error { return errors.New("timeout") })
	fmt.Println(err)
	err = b.Do(context.Background(), func(context.Context) error { return nil })
	if errors.Is(err, breaker.ErrOpen) {
		fmt.Println("serving the cached value")
	}
	// Output:
	// timeout
	// serving the cached value
}

func ExampleBreaker_State() {
	f := clock.NewFake(start)
	b := breaker.New(breaker.WithFailureThreshold(1), breaker.WithOpenFor(time.Minute), breaker.WithClock(f))
	fmt.Println(b.State())
	_ = b.Do(context.Background(), func(context.Context) error { return errors.New("down") })
	fmt.Println(b.State())
	f.Advance(time.Minute)
	fmt.Println(b.State())
	// Output:
	// closed
	// open
	// half-open
}

func ExampleDoValue() {
	b := breaker.New()
	price, err := breaker.DoValue(context.Background(), b, func(context.Context) (float64, error) {
		return 9.99, nil
	})
	fmt.Println(price, err)
	// Output:
	// 9.99 <nil>
}

func ExampleState() {
	for _, s := range []breaker.State{breaker.Closed, breaker.Open, breaker.HalfOpen} {
		fmt.Println(int(s), s)
	}
	// Output:
	// 0 closed
	// 1 open
	// 2 half-open
}

func ExampleErrOpen() {
	b := breaker.New(breaker.WithFailureThreshold(1))
	_ = b.Do(context.Background(), func(context.Context) error { return errors.New("down") })
	called := false
	err := b.Do(context.Background(), func(context.Context) error { called = true; return nil })
	fmt.Println(errors.Is(err, breaker.ErrOpen), called)
	// Output:
	// true false
}

func ExampleOption() {
	opts := []breaker.Option{breaker.WithFailureThreshold(3), breaker.WithHalfOpenProbes(2)}
	b := breaker.New(opts...)
	fmt.Println(b.State())
	// Output:
	// closed
}

func ExampleWithFailureThreshold() {
	b := breaker.New(breaker.WithFailureThreshold(3))
	for i := 1; i <= 3; i++ {
		_ = b.Do(context.Background(), func(context.Context) error { return errors.New("down") })
		fmt.Println(i, b.State())
	}
	// Output:
	// 1 closed
	// 2 closed
	// 3 open
}

func ExampleWithOpenFor() {
	f := clock.NewFake(start)
	b := breaker.New(breaker.WithFailureThreshold(1), breaker.WithOpenFor(10*time.Second), breaker.WithClock(f))
	_ = b.Do(context.Background(), func(context.Context) error { return errors.New("down") })
	f.Advance(9 * time.Second)
	fmt.Println(b.State())
	f.Advance(time.Second)
	fmt.Println(b.State())
	// Output:
	// open
	// half-open
}

func ExampleWithHalfOpenProbes() {
	f := clock.NewFake(start)
	b := breaker.New(breaker.WithFailureThreshold(1), breaker.WithOpenFor(time.Second),
		breaker.WithHalfOpenProbes(2), breaker.WithClock(f))
	_ = b.Do(context.Background(), func(context.Context) error { return errors.New("down") })
	f.Advance(time.Second)
	for range 2 {
		_ = b.Do(context.Background(), func(context.Context) error { return nil })
		fmt.Println(b.State())
	}
	// Output:
	// half-open
	// closed
}

func ExampleWithClock() {
	f := clock.NewFake(start) // production passes clock.Real(), the default
	b := breaker.New(breaker.WithFailureThreshold(1), breaker.WithOpenFor(time.Hour), breaker.WithClock(f))
	_ = b.Do(context.Background(), func(context.Context) error { return errors.New("down") })
	f.Advance(time.Hour)
	fmt.Println(b.State())
	// Output:
	// half-open
}

func ExampleWithOnStateChange() {
	f := clock.NewFake(start)
	b := breaker.New(
		breaker.WithFailureThreshold(1),
		breaker.WithOpenFor(time.Second),
		breaker.WithClock(f),
		breaker.WithOnStateChange(func(from, to breaker.State) { fmt.Println(from, "->", to) }),
	)
	_ = b.Do(context.Background(), func(context.Context) error { return errors.New("down") })
	f.Advance(time.Second)
	_ = b.Do(context.Background(), func(context.Context) error { return nil })
	// Output:
	// closed -> open
	// open -> half-open
	// half-open -> closed
}
