package schedule_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/schedule"
)

func ExampleOption() {
	opts := []schedule.Option{
		schedule.WithJitter(0.1),
		nil, // ignored
	}
	err := schedule.Run(context.Background(), schedule.Every(0), func(context.Context) error { return nil }, opts...)
	fmt.Println(err)
	// Output: <nil>
}

func ExampleWithClock() {
	f := clock.NewFake(start)
	errc := make(chan error, 1)
	go func() {
		errc <- schedule.Run(context.Background(), schedule.At(start.Add(time.Hour)), func(context.Context) error {
			fmt.Println("ran at", f.Now().Format(time.TimeOnly))
			return nil
		}, schedule.WithClock(f))
	}()
	_ = f.BlockUntil(context.Background(), 1)
	f.Advance(time.Hour)
	fmt.Println(<-errc)
	// Output:
	// ran at 01:00:00
	// <nil>
}

func ExampleWithJitter() {
	rc := newRec()
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	errc := make(chan error, 1)
	go func() {
		errc <- schedule.Run(ctx, schedule.Every(time.Minute), func(context.Context) error {
			if runs++; runs == 5 {
				cancel()
			}
			return nil
		}, schedule.WithClock(rc), schedule.WithJitter(0.1), schedule.WithRand(rand.New(rand.NewPCG(7, 7))))
	}()
	within := true
	for range 5 {
		d := <-rc.waits // the first wait is from start; later ones from the previous run
		within = within && d > 0 && d < 2*time.Minute
		rc.Advance(d)
	}
	<-errc
	fmt.Println("waits jittered and bounded:", within)
	// Output: waits jittered and bounded: true
}

func ExampleWithOnError() {
	f := clock.NewFake(start)
	ctx, cancel := context.WithCancel(context.Background())
	failures := 0
	onError := func(err error) {
		failures++
		fmt.Printf("run %d failed: %v\n", failures, err)
		if failures == 2 {
			cancel()
		}
	}
	errc := make(chan error, 1)
	go func() {
		errc <- schedule.Run(ctx, schedule.Every(time.Second), func(context.Context) error { return errors.New("boom") },
			schedule.WithClock(f), schedule.WithOnError(onError))
	}()
	for range 2 {
		_ = f.BlockUntil(ctx, 1)
		f.Advance(time.Second)
	}
	fmt.Println(<-errc)
	// Output:
	// run 1 failed: boom
	// run 2 failed: boom
	// context canceled
}

func ExampleWithRand() {
	waits := func() []time.Duration {
		rc := newRec()
		ctx, cancel := context.WithCancel(context.Background())
		runs := 0
		errc := make(chan error, 1)
		go func() {
			errc <- schedule.Run(ctx, schedule.Every(time.Second), func(context.Context) error {
				if runs++; runs == 3 {
					cancel()
				}
				return nil
			}, schedule.WithClock(rc), schedule.WithJitter(0.5), schedule.WithRand(rand.New(rand.NewPCG(1, 2))))
		}()
		var ds []time.Duration
		for range 3 {
			d := <-rc.waits
			ds = append(ds, d)
			rc.Advance(d)
		}
		<-errc
		return ds
	}
	fmt.Println("same seed, same waits:", fmt.Sprint(waits()) == fmt.Sprint(waits()))
	// Output: same seed, same waits: true
}
