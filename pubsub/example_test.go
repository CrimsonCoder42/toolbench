package pubsub_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/pubsub"
)

// readyHook returns a WithOnSubscribers option that signals on the returned channel each time topic's
// subscriber count rises to want, so an example publishes only once its subscribers are registered.
func readyHook[M any](want int) (pubsub.Option[M], <-chan struct{}) {
	ready := make(chan struct{}, 1)
	return pubsub.WithOnSubscribers[M](func(_ string, n int) {
		if n == want {
			ready <- struct{}{}
		}
	}), ready
}

func ExampleBus() {
	hook, ready := readyHook[string](2)
	bus := pubsub.NewBus[string](hook)
	errStop := errors.New("stop")
	var gotA, gotB []string
	doneA, doneB := make(chan error, 1), make(chan error, 1)
	subscribe := func(got *[]string, done chan<- error) {
		done <- bus.Subscribe(context.Background(), "orders", func(_ context.Context, m string) error {
			*got = append(*got, m)
			if len(*got) == 3 {
				return errStop
			}
			return nil
		})
	}
	go subscribe(&gotA, doneA)
	go subscribe(&gotB, doneB)
	<-ready
	err := bus.Publish(context.Background(), "orders", "latte", "mocha", "flat white")
	errA, errB := <-doneA, <-doneB
	fmt.Println(err, gotA, gotB, errors.Is(errA, errStop), errors.Is(errB, errStop))
	// Output: <nil> [latte mocha flat white] [latte mocha flat white] true true
}

func ExampleNewBus() {
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](8))
	defer bus.Close()
	fmt.Println(bus.Publish(context.Background(), "nobody-listens", 1, 2, 3))
	// Output: <nil>
}

func ExampleBus_Publish() {
	var bus pubsub.Bus[string] // the zero value is ready to use
	fmt.Println(bus.Publish(context.Background(), "news", "no subscriber: delivered to nobody"))
	_ = bus.Close()
	fmt.Println(bus.Publish(context.Background(), "news", "after Close"))
	// Output:
	// <nil>
	// pubsub: bus closed
}

func ExampleBus_Subscribe() {
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](hook)
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(context.Background(), "jobs", func(_ context.Context, n int) error {
			if n < 0 {
				return fmt.Errorf("negative job %d", n)
			}
			return nil
		})
	}()
	<-ready
	_ = bus.Publish(context.Background(), "jobs", 1, -2)
	fmt.Println(<-done) // the handler's error ends the subscription
	// Output: negative job -2
}

func ExampleBus_Close() {
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](hook)
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(context.Background(), "t", func(context.Context, int) error { return nil })
	}()
	<-ready
	fmt.Println(bus.Close())
	err := <-done
	fmt.Println(errors.Is(err, pubsub.ErrClosed), bus.Close())
	// Output:
	// <nil>
	// true <nil>
}

// notify takes a pubsub.Publisher: it works with the in-process Bus and with any broker adapter.
func notify(ctx context.Context, p pubsub.Publisher[string], user string) error {
	return p.Publish(ctx, "signups", "welcome "+user)
}

func ExamplePublisher() {
	var bus pubsub.Bus[string]
	fmt.Println(notify(context.Background(), &bus, "ada"))
	// Output: <nil>
}

func ExampleSubscriber() {
	var s pubsub.Subscriber[string] = pubsub.NewBus[string]()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // unsubscribing is cancelling ctx
	err := s.Subscribe(ctx, "signups", func(context.Context, string) error { return nil })
	fmt.Println(errors.Is(err, context.Canceled))
	// Output: true
}

func ExampleOption() {
	opts := []pubsub.Option[float64]{pubsub.WithBuffer[float64](16), pubsub.WithDropOldest(func(string, float64) {})}
	bus := pubsub.NewBus[float64](opts...)
	fmt.Println(bus.Publish(context.Background(), "prices", 1.5))
	// Output: <nil>
}

func ExampleWithBuffer() {
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](3), hook)
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	var got []int
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(ctx, "t", func(_ context.Context, n int) error {
			<-release // the handler is held: the buffer absorbs what Publish sends
			got = append(got, n)
			if len(got) == 3 {
				cancel()
			}
			return nil
		})
	}()
	<-ready
	err := bus.Publish(ctx, "t", 1, 2, 3) // returns although the handler has not run: buffered
	close(release)
	<-done
	fmt.Println(err, got)
	// Output: <nil> [1 2 3]
}

func ExampleWithDropOldest() {
	var dropped []int
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](hook, pubsub.WithBuffer[int](1),
		pubsub.WithDropOldest(func(_ string, n int) { dropped = append(dropped, n) }))
	ctx, cancel := context.WithCancel(context.Background())
	started, release := make(chan struct{}), make(chan struct{})
	var got []int
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(ctx, "ticks", func(_ context.Context, n int) error {
			got = append(got, n)
			if n == 1 {
				close(started)
				<-release // a slow subscriber
			}
			if n == 4 {
				cancel()
			}
			return nil
		})
	}()
	<-ready
	_ = bus.Publish(ctx, "ticks", 1)
	<-started
	_ = bus.Publish(ctx, "ticks", 2, 3, 4) // never blocks: the buffer of 1 keeps the newest
	close(release)
	<-done
	fmt.Println("dropped", dropped, "got", got)
	// Output: dropped [2 3] got [1 4]
}

func ExampleWithOnSubscribers() {
	var counts []int
	bus := pubsub.NewBus[int](pubsub.WithOnSubscribers[int](func(_ string, n int) { counts = append(counts, n) }))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = bus.Subscribe(ctx, "t", func(context.Context, int) error { return nil }) // ctx already done: never registers
	bus2 := pubsub.NewBus[int](pubsub.WithOnSubscribers[int](func(_ string, n int) { counts = append(counts, n) }))
	_ = bus2.Close()
	_ = bus2.Subscribe(context.Background(), "t", func(context.Context, int) error { return nil })
	fmt.Println(len(counts))

	hook := make(chan int, 2)
	bus3 := pubsub.NewBus[int](pubsub.WithOnSubscribers[int](func(_ string, n int) { hook <- n }))
	done := make(chan error, 1)
	go func() {
		done <- bus3.Subscribe(context.Background(), "t", func(context.Context, int) error { return errors.New("bye") })
	}()
	joined := <-hook
	_ = bus3.Publish(context.Background(), "t", 1)
	<-done
	fmt.Println(joined, <-hook)
	// Output:
	// 0
	// 1 0
}

func ExampleErrClosed() {
	bus := pubsub.NewBus[int]()
	_ = bus.Close()
	err := bus.Subscribe(context.Background(), "t", func(context.Context, int) error { return nil })
	fmt.Println(errors.Is(err, pubsub.ErrClosed))
	// Output: true
}

func ExampleErrInvalid() {
	bus := pubsub.NewBus[string](pubsub.WithBuffer[string](1<<20 + 1)) // above the 2^20 bound
	err := bus.Publish(context.Background(), "t", "x")
	fmt.Println(errors.Is(err, pubsub.ErrInvalid))
	err = pubsub.NewBus[string]().Subscribe(context.Background(), "t", nil)
	fmt.Println(errors.Is(err, pubsub.ErrInvalid))
	// Output:
	// true
	// true
}

// A panic in a handler ends that subscription with panics.ErrPanic; the Bus and the process keep running.
func ExampleBus_Subscribe_panic() {
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](hook)
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(context.Background(), "t", func(context.Context, int) error { panic("boom") })
	}()
	<-ready
	_ = bus.Publish(context.Background(), "t", 1)
	fmt.Println(errors.Is(<-done, panics.ErrPanic))
	// Output: true
}

// A request id set by the publisher travels in the headers; the handler that reacts republishes on another
// topic with that id as the correlation id (WithHeader copies: the shared map of the order is untouched).
func ExampleMessage() {
	type Order = pubsub.Message[string]
	ready := make(chan struct{}, 2)
	bus := pubsub.NewBus[Order](pubsub.WithOnSubscribers[Order](func(_ string, n int) {
		if n == 1 {
			ready <- struct{}{}
		}
	}))
	done := errors.New("done")
	invoices := make(chan error, 1)
	go func() {
		invoices <- bus.Subscribe(context.Background(), "invoices", func(_ context.Context, m Order) error {
			id, _ := m.Header(pubsub.HeaderCorrelationID)
			fmt.Println("invoices:", m.Body, "correlation-id", id)
			return done
		})
	}()
	orders := make(chan error, 1)
	go func() {
		orders <- bus.Subscribe(context.Background(), "orders", func(ctx context.Context, m Order) error {
			reqID, _ := m.Header(pubsub.HeaderRequestID)
			invoice := m.WithHeader(pubsub.HeaderCorrelationID, reqID)
			invoice.Body = "invoice for " + m.Body
			if err := bus.Publish(ctx, "invoices", invoice); err != nil {
				return err
			}
			return done
		})
	}()
	<-ready
	<-ready
	order := Order{Body: "latte"}.WithHeader(pubsub.HeaderRequestID, "req-42")
	err := bus.Publish(context.Background(), "orders", order)
	<-orders
	<-invoices
	_, hasCorrelation := order.Header(pubsub.HeaderCorrelationID)
	fmt.Println(err, "order carries a correlation-id:", hasCorrelation)
	// Output:
	// invoices: invoice for latte correlation-id req-42
	// <nil> order carries a correlation-id: false
}

func ExampleMessage_Header() {
	m := pubsub.Message[int]{Headers: map[string]string{pubsub.HeaderRequestID: "req-1"}, Body: 7}
	id, ok := m.Header(pubsub.HeaderRequestID)
	_, hasCorrelation := m.Header(pubsub.HeaderCorrelationID)
	fmt.Println(id, ok, hasCorrelation)
	// Output: req-1 true false
}

func ExampleMessage_WithHeader() {
	m := pubsub.Message[int]{Body: 1} // the zero Headers map is fine
	m2 := m.WithHeader(pubsub.HeaderRequestID, "req-1")
	fmt.Println(m.Headers == nil, len(m2.Headers), m2.Body)
	// Output: true 1 1
}

func ExampleHeaderRequestID() {
	fmt.Println(pubsub.HeaderRequestID)
	// Output: request-id
}

func ExampleHeaderCorrelationID() {
	fmt.Println(pubsub.HeaderCorrelationID)
	// Output: correlation-id
}

// A slow subscriber is evicted (its ctx cancelled with cause ErrSlowSubscriber) and the publisher never waits.
func ExampleWithDropSubscriber() {
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](hook, pubsub.WithBuffer[int](1),
		pubsub.WithDropSubscriber(func(topic string, n int) { fmt.Println("evicted a subscriber of", topic, "at message", n) }))
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(context.Background(), "ticks", func(context.Context, int) error {
			close(started)
			<-release // a slow subscriber
			return nil
		})
	}()
	<-ready
	_ = bus.Publish(context.Background(), "ticks", 1) // taken by the handler
	<-started
	err := bus.Publish(context.Background(), "ticks", 2, 3) // 2 fills the buffer of 1; 3 finds it full
	close(release)
	fmt.Println(err, "|", <-done)
	// Output:
	// evicted a subscriber of ticks at message 3
	// <nil> | pubsub: 1 buffered messages not delivered: pubsub: slow subscriber
}

func ExampleErrSlowSubscriber() {
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](hook, pubsub.WithDropSubscriber[int](nil)) // nil report: evict silently
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(context.Background(), "t", func(ctx context.Context, _ int) error {
			close(started)
			<-release
			<-ctx.Done()
			fmt.Println("handler ctx cause:", context.Cause(ctx))
			return nil
		})
	}()
	<-ready
	_ = bus.Publish(context.Background(), "t", 1) // taken by the handler
	<-started
	_ = bus.Publish(context.Background(), "t", 2) // kept (no buffer means 1)
	_ = bus.Publish(context.Background(), "t", 3) // full: evicted
	close(release)
	fmt.Println(errors.Is(<-done, pubsub.ErrSlowSubscriber))
	// Output:
	// handler ctx cause: pubsub: slow subscriber
	// true
}

func ExampleWithOnDelivered() {
	hook, ready := readyHook[int](1)
	bus := pubsub.NewBus[int](hook, pubsub.WithOnDelivered(func(topic string, n int, _ time.Duration, err error) {
		fmt.Printf("%s %d err=%v\n", topic, n, err)
	}))
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(context.Background(), "jobs", func(_ context.Context, n int) error {
			if n == 2 {
				return errors.New("bad job")
			}
			return nil
		})
	}()
	<-ready
	_ = bus.Publish(context.Background(), "jobs", 1, 2)
	fmt.Println(<-done)
	// Output:
	// jobs 1 err=<nil>
	// jobs 2 err=bad job
	// bad job
}

func ExampleWithClock() {
	fake := clock.NewFake(time.Unix(0, 0))
	hook, ready := readyHook[string](1)
	bus := pubsub.NewBus[string](hook, pubsub.WithClock[string](fake),
		pubsub.WithOnDelivered(func(_ string, m string, d time.Duration, _ error) { fmt.Println(m, "took", d) }))
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(context.Background(), "t", func(context.Context, string) error {
			fake.Advance(250 * time.Millisecond) // the handler's work, on the fake clock
			return errors.New("stop")
		})
	}()
	<-ready
	_ = bus.Publish(context.Background(), "t", "render")
	<-done
	// Output: render took 250ms
}
