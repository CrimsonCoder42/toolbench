package ratelimit_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/ratelimit"
)

func ExampleLimit() {
	r := ratelimit.Limit{Events: 10, Per: time.Second} // x/time/rate's rate.Limit(10), exactly
	fmt.Println(r.Events, r.Per)
	// Output: 10 1s
}

func ExampleEvery() {
	fmt.Printf("%+v\n", ratelimit.Every(200*time.Millisecond))
	// Output: {Events:1 Per:200ms}
}

func ExampleNewLimiter() {
	_, err := ratelimit.NewLimiter(ratelimit.Every(0), 1)
	fmt.Println(errors.Is(err, ratelimit.ErrLimit))
	// Output: true
}

func ExampleNewKeyed() {
	_, err := ratelimit.NewKeyed[string](ratelimit.Every(time.Second), 0)
	fmt.Println(errors.Is(err, ratelimit.ErrBurst))
	// Output: true
}

func ExampleLimiter() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l, _ := ratelimit.NewLimiter(ratelimit.Limit{Events: 2, Per: time.Second}, 3, ratelimit.WithClock(f))
	fmt.Println(l.Allow(), l.Allow(), l.Allow(), l.Allow()) // the burst, then empty
	f.Advance(500 * time.Millisecond)
	fmt.Println(l.Allow()) // one token every 500 ms
	// Output:
	// true true true false
	// true
}

func ExampleLimiter_Reserve() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l, _ := ratelimit.NewLimiter(ratelimit.Every(time.Second), 1, ratelimit.WithClock(f))
	for range 3 {
		r := l.Reserve()
		fmt.Println(r.OK(), r.Delay())
	}
	// Output:
	// true 0s
	// true 1s
	// true 2s
}

func ExampleLimiter_Wait() {
	l, _ := ratelimit.NewLimiter(ratelimit.Every(time.Second), 2)
	fmt.Println(l.Wait(context.Background())) // a token is there: no wait
	err := l.WaitN(context.Background(), 3)
	fmt.Println(errors.Is(err, ratelimit.ErrBurst))
	// Output:
	// <nil>
	// true
}

func ExampleReservation_Cancel() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l, _ := ratelimit.NewLimiter(ratelimit.Every(time.Second), 1, ratelimit.WithClock(f))
	l.Allow()
	r := l.Reserve()
	fmt.Println(r.Delay())
	r.Cancel() // will not act: the token goes back
	fmt.Println(l.Reserve().Delay())
	// Output:
	// 1s
	// 1s
}

func ExampleKeyed() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	k, _ := ratelimit.NewKeyed[string](ratelimit.Every(time.Second), 1, ratelimit.WithClock(f))
	fmt.Println(k.Allow("10.0.0.1"))
	fmt.Println(k.Allow("10.0.0.1")) // retry-after for a 429
	fmt.Println(k.Allow("10.0.0.2"))
	// Output:
	// true 0s
	// false 1s
	// true 0s
}

func ExampleWithClock() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l, _ := ratelimit.NewLimiter(ratelimit.Every(time.Minute), 1, ratelimit.WithClock(f))
	l.Allow()
	f.Advance(time.Minute)
	fmt.Println(l.Allow())
	// Output: true
}

func ExampleWithMaxKeys() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	k, _ := ratelimit.NewKeyed[string](ratelimit.Every(time.Second), 1,
		ratelimit.WithClock(f), ratelimit.WithMaxKeys(1))
	k.Allow("a")
	_, retry, err := k.AllowN("b", 1)
	fmt.Println(retry, errors.Is(err, ratelimit.ErrKeys))
	// Output: 1s true
}

func ExampleWithIdleTTL() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	k, _ := ratelimit.NewKeyed[string](ratelimit.Every(time.Second), 1,
		ratelimit.WithClock(f), ratelimit.WithIdleTTL(time.Hour))
	k.Allow("a")
	f.Advance(59 * time.Minute)
	fmt.Println(k.Len())
	f.Advance(time.Minute)
	fmt.Println(k.Len())
	// Output:
	// 1
	// 0
}

func ExampleErrBurst() {
	k, _ := ratelimit.NewKeyed[string](ratelimit.Every(time.Second), 2)
	_, _, err := k.AllowN("a", 3)
	fmt.Println(errors.Is(err, ratelimit.ErrBurst))
	// Output: true
}

func ExampleErrKeys() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	k, _ := ratelimit.NewKeyed[int](ratelimit.Every(time.Second), 1, ratelimit.WithClock(f), ratelimit.WithMaxKeys(1))
	k.Allow(1)
	_, _, err := k.AllowN(2, 1)
	fmt.Println(err)
	// Output: ratelimit: key bound full
}

func ExampleErrLimit() {
	_, err := ratelimit.NewKeyed[string](ratelimit.Limit{Events: 0, Per: time.Second}, 1)
	fmt.Println(errors.Is(err, ratelimit.ErrLimit))
	// Output: true
}

func ExampleInfDuration() {
	l, _ := ratelimit.NewLimiter(ratelimit.Every(time.Second), 1)
	fmt.Println(l.ReserveN(2).Delay() == ratelimit.InfDuration)
	// Output: true
}

func ExampleDefaultMaxKeys() {
	fmt.Println(ratelimit.DefaultMaxKeys)
	// Output: 10000
}

func ExampleOption() {
	opts := []ratelimit.Option{ratelimit.WithMaxKeys(100), ratelimit.WithIdleTTL(time.Minute)}
	k, err := ratelimit.NewKeyed[string](ratelimit.Every(time.Second), 5, opts...)
	fmt.Println(k.Burst(), err)
	// Output: 5 <nil>
}
