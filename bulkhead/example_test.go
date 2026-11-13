package bulkhead_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/bulkhead"
	"github.com/CrimsonCoder42/toolbench/clock"
)

// busy starts a call that holds one slot of b until the returned func is called; it returns once fn runs.
func busy(b *bulkhead.Bulkhead) (release func()) {
	entered, rel, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = b.Do(context.Background(), func(context.Context) error {
			close(entered)
			<-rel
			return nil
		})
	}()
	<-entered
	return func() { close(rel); <-done }
}

func ExampleBulkhead() {
	b, _ := bulkhead.New(1) // one call at a time, no queue: fail fast
	release := busy(b)
	err := b.Do(context.Background(), func(context.Context) error { return nil })
	fmt.Println(err, errors.Is(err, bulkhead.ErrFull))
	release()
	fmt.Println(b.Do(context.Background(), func(context.Context) error { return nil }))
	// Output:
	// bulkhead: full true
	// <nil>
}

func ExampleBulkhead_Do() {
	b, _ := bulkhead.New(2)
	err := b.Do(context.Background(), func(ctx context.Context) error {
		return errors.New("upstream said no")
	})
	fmt.Println(err)
	// Output: upstream said no
}

func ExampleNew() {
	_, err := bulkhead.New(0)
	fmt.Println(err)
	b, err := bulkhead.New(8, bulkhead.WithQueue(16), bulkhead.WithQueueTimeout(time.Second))
	fmt.Println(b != nil, err)
	// Output:
	// bulkhead: maxConcurrent must be > 0: got 0
	// true <nil>
}

func ExampleDoValue() {
	b, _ := bulkhead.New(4)
	price, err := bulkhead.DoValue(context.Background(), b, func(context.Context) (float64, error) {
		return 9.99, nil
	})
	fmt.Println(price, err)
	// Output: 9.99 <nil>
}

func ExampleErrFull() {
	b, _ := bulkhead.New(1)
	release := busy(b)
	defer release()
	if err := b.Do(context.Background(), func(context.Context) error { return nil }); errors.Is(err, bulkhead.ErrFull) {
		fmt.Println("shed: serve the cached answer")
	}
	// Output: shed: serve the cached answer
}

func ExampleErrQueueTimeout() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	queued := make(chan struct{})
	b, _ := bulkhead.New(1,
		bulkhead.WithQueue(1),
		bulkhead.WithQueueTimeout(time.Second),
		bulkhead.WithClock(f),
		bulkhead.WithOnEvent(func(e bulkhead.Event) {
			if e.Kind == bulkhead.EventQueued {
				close(queued)
			}
		}),
	)
	release := busy(b)
	defer release()
	done := make(chan error)
	go func() { done <- b.Do(context.Background(), func(context.Context) error { return nil }) }()
	<-queued
	f.Advance(time.Second)
	err := <-done
	fmt.Println(err)
	fmt.Println(errors.Is(err, bulkhead.ErrQueueTimeout), errors.Is(err, bulkhead.ErrFull))
	// Output:
	// bulkhead: queue timeout: bulkhead: full
	// true true
}

func ExampleErrMaxConcurrent() {
	_, err := bulkhead.New(-1)
	fmt.Println(errors.Is(err, bulkhead.ErrMaxConcurrent))
	// Output: true
}

func ExampleOption() {
	opts := []bulkhead.Option{bulkhead.WithQueue(2), nil} // a nil Option is ignored
	b, err := bulkhead.New(1, opts...)
	fmt.Println(b != nil, err)
	// Output: true <nil>
}

func ExampleWithQueue() {
	var order []int
	queued := make(chan struct{}, 2)
	b, _ := bulkhead.New(1, bulkhead.WithQueue(2), bulkhead.WithOnEvent(func(e bulkhead.Event) {
		if e.Kind == bulkhead.EventQueued {
			queued <- struct{}{}
		}
	}))
	release := busy(b)
	done := make(chan struct{})
	for i := range 2 {
		go func() {
			_ = b.Do(context.Background(), func(context.Context) error { order = append(order, i); return nil })
			done <- struct{}{}
		}()
		<-queued // call i is in the queue before call i+1 starts
	}
	err := b.Do(context.Background(), func(context.Context) error { return nil })
	fmt.Println("third waiter:", err)
	release()
	<-done
	<-done
	fmt.Println("ran in order:", order)
	// Output:
	// third waiter: bulkhead: full
	// ran in order: [0 1]
}

func ExampleWithQueueTimeout() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	waited := make(chan time.Duration, 1)
	queued := make(chan struct{})
	b, _ := bulkhead.New(1,
		bulkhead.WithQueue(1),
		bulkhead.WithQueueTimeout(500*time.Millisecond),
		bulkhead.WithClock(f),
		bulkhead.WithOnEvent(func(e bulkhead.Event) {
			switch e.Kind {
			case bulkhead.EventQueued:
				close(queued)
			case bulkhead.EventTimedOut:
				waited <- e.Waited
			}
		}),
	)
	release := busy(b)
	defer release()
	done := make(chan error)
	go func() { done <- b.Do(context.Background(), func(context.Context) error { return nil }) }()
	<-queued
	f.Advance(500 * time.Millisecond)
	fmt.Println(<-done, <-waited)
	// Output: bulkhead: queue timeout: bulkhead: full 500ms
}

func ExampleWithClock() {
	f := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b, _ := bulkhead.New(1, bulkhead.WithClock(f))
	fmt.Println(b.Do(context.Background(), func(context.Context) error { return nil }))
	// Output: <nil>
}

func ExampleWithOnEvent() {
	b, _ := bulkhead.New(1, bulkhead.WithOnEvent(func(e bulkhead.Event) { fmt.Println(e.Kind) }))
	release := busy(b)
	_ = b.Do(context.Background(), func(context.Context) error { return nil })
	release()
	// Output:
	// admitted
	// rejected
	// released
}

func ExampleEvent() {
	var last bulkhead.Event
	b, _ := bulkhead.New(1, bulkhead.WithOnEvent(func(e bulkhead.Event) {
		if e.Kind == bulkhead.EventAdmitted {
			last = e
		}
	}))
	_ = b.Do(context.Background(), func(context.Context) error { return nil })
	fmt.Println(last.Kind, last.Waited)
	// Output: admitted 0s
}

func ExampleEventKind() {
	fmt.Println(bulkhead.EventAdmitted, bulkhead.EventRejected, bulkhead.EventQueued,
		bulkhead.EventTimedOut, bulkhead.EventCancelled, bulkhead.EventReleased)
	// Output: admitted rejected queued timed-out cancelled released
}
