package pubsub_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/pubsub"
)

type delivery struct {
	topic string
	m     int
	d     time.Duration
	err   error
}

// The hook is called exactly once per handler call, after it, with the handler's result: nil, its error, or
// ErrPanic when it panicked.
func TestWithOnDelivered_calledOncePerDeliveryWithHandlerResult(t *testing.T) {
	leak.Verify(t)
	for _, tc := range []struct {
		name    string
		h       func(context.Context, int) error
		want    []error // per delivery, matched with errors.Is; nil = success
		wantSub error
	}{
		{"handlerErrorEnds", func(_ context.Context, n int) error {
			if n == 3 {
				return errA
			}
			return nil
		}, []error{nil, nil, errA}, errA},
		{"handlerPanic", func(_ context.Context, n int) error {
			if n == 2 {
				panic("boom")
			}
			return nil
		}, []error{nil, panics.ErrPanic}, panics.ErrPanic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := bounded(t)
			deliveries := make(chan delivery, 8)
			hook, ready := counter[int](1)
			bus := pubsub.NewBus[int](hook, pubsub.WithOnDelivered(func(topic string, m int, d time.Duration, err error) {
				deliveries <- delivery{topic, m, d, err}
			}))
			done := subscribe(bus, ctx, "t", tc.h)
			recv(t, ctx, ready)
			if err := bus.Publish(ctx, "t", 1, 2, 3); err != nil {
				t.Fatal(err)
			}
			if err := recv(t, ctx, done); !errors.Is(err, tc.wantSub) {
				t.Fatalf("Subscribe: %v", err)
			}
			for i, want := range tc.want {
				got := recv(t, ctx, deliveries)
				if got.topic != "t" || got.m != i+1 || got.d < 0 {
					t.Fatalf("delivery %d: %+v", i, got)
				}
				if want == nil && got.err != nil || want != nil && !errors.Is(got.err, want) {
					t.Fatalf("delivery %d: err %v, want %v", i, got.err, want)
				}
			}
			select {
			case d := <-deliveries:
				t.Fatalf("extra hook call %+v", d)
			default:
			}
		})
	}
}

func TestWithOnDelivered_durationFromFakeClock(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	fake := clock.NewFake(time.Unix(0, 0))
	durations := make(chan time.Duration, 2)
	hook, ready := counter[int](1)
	bus := pubsub.NewBus[int](hook, pubsub.WithClock[int](fake),
		pubsub.WithOnDelivered(func(_ string, _ int, d time.Duration, _ error) { durations <- d }))
	done := subscribe(bus, ctx, "t", func(_ context.Context, n int) error {
		fake.Advance(time.Duration(n) * 10 * time.Millisecond) // the handler "takes" n*10ms
		if n == 2 {
			return errStop
		}
		return nil
	})
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "t", 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, ctx, done); !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if d := recv(t, ctx, durations); d != 10*time.Millisecond {
		t.Fatalf("first duration %v", d)
	}
	if d := recv(t, ctx, durations); d != 20*time.Millisecond {
		t.Fatalf("second duration %v", d)
	}
}

// A panic in the hook ends that Subscribe with ErrPanic (the handler ran once); the Bus keeps working.
func TestWithOnDelivered_hookPanicEndsSubscribe(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	hook, ready := counter[int](1)
	var calls atomic.Int64
	bus := pubsub.NewBus[int](hook, pubsub.WithOnDelivered(func(string, int, time.Duration, error) { panic("hook") }))
	done := subscribe(bus, ctx, "t", func(context.Context, int) error { calls.Add(1); return nil })
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "t", 1); err != nil {
		t.Fatal(err)
	}
	err := recv(t, ctx, done)
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "hook" {
		t.Fatalf("Subscribe: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("handler ran %d times", calls.Load())
	}
	done = subscribe(bus, ctx, "t", func(context.Context, int) error { calls.Add(1); return errStop })
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "t", 2); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, ctx, done); !errors.Is(err, panics.ErrPanic) || calls.Load() != 2 {
		t.Fatalf("second subscriber: %v after %d calls", err, calls.Load())
	}
}

func TestWithClock_nilIsReal(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	durations := make(chan time.Duration, 1)
	hook, ready := counter[int](1)
	bus := pubsub.NewBus[int](hook, pubsub.WithClock[int](nil),
		pubsub.WithOnDelivered(func(_ string, _ int, d time.Duration, _ error) { durations <- d }))
	done := subscribe(bus, ctx, "t", func(context.Context, int) error { return errStop })
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "t", 1); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, ctx, done); !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if d := recv(t, ctx, durations); d < 0 {
		t.Fatalf("real clock measured %v", d)
	}
}
