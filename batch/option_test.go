package batch_test

import (
	"context"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/batch"
	"github.com/CrimsonCoder42/toolbench/clock"
)

// A partial batch is sent maxWait after its first value; a fake clock makes that step deterministic.
func ExampleWithClock() {
	ctx := context.Background()
	fake := clock.NewFake(time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC))
	in := make(chan string)
	out := batch.Channel(ctx, in, 100, time.Second, batch.WithClock(fake))

	in <- "a"
	in <- "b"
	_ = fake.BlockUntil(ctx, 1) // the batch timer is armed
	fake.Advance(time.Second)
	fmt.Println(<-out)
	close(in)
	for b := range out {
		fmt.Println("unexpected", b)
	}
	// Output:
	// [a b]
}

func ExampleOption() {
	opts := []batch.Option{batch.WithClock(clock.Real()), nil} // a nil Option is ignored
	for b := range batch.Channel(context.Background(), filled(1, 2, 3), 3, time.Minute, opts...) {
		fmt.Println(b)
	}
	// Output:
	// [1 2 3]
}
