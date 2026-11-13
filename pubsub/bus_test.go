package pubsub_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/pubsub"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var (
	errStop = errors.New("stop")
	errA    = errors.New("a failed")
)

// bounded returns a context that ends the test if a wait hangs.
func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// recv waits for one value on ch or fails the test when ctx ends.
func recv[T any](t *testing.T, ctx context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-ctx.Done():
		t.Fatalf("timed out waiting: %v", context.Cause(ctx))
		var zero T
		return zero
	}
}

// subscribe runs bus.Subscribe on a new goroutine and returns the channel its result arrives on.
func subscribe[M any](bus *pubsub.Bus[M], ctx context.Context, topic string, h func(context.Context, M) error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- bus.Subscribe(ctx, topic, h) }()
	return done
}

// counter is a WithOnSubscribers hook that signals each time any topic's count reaches want.
func counter[M any](want int) (pubsub.Option[M], <-chan struct{}) {
	ready := make(chan struct{}, 64)
	return pubsub.WithOnSubscribers[M](func(_ string, n int) {
		if n == want {
			ready <- struct{}{}
		}
	}), ready
}

// collectUntil returns a handler that appends each message to *got and returns errStop after the n-th.
func collectUntil[M any](got *[]M, n int) func(context.Context, M) error {
	return func(_ context.Context, m M) error {
		*got = append(*got, m)
		if len(*got) == n {
			return errStop
		}
		return nil
	}
}

func seq(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func TestBus_fanOutToEverySubscriber(t *testing.T) {
	leak.Verify(t)
	for _, tc := range []struct {
		name string
		opts []pubsub.Option[int]
	}{
		{"direct", nil},
		{"buffer4", []pubsub.Option[int]{pubsub.WithBuffer[int](4)}},
		{"dropOldestRoomForAll", []pubsub.Option[int]{pubsub.WithBuffer[int](200), pubsub.WithDropOldest(func(string, int) {})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := bounded(t)
			const subs, n = 3, 100
			hook, ready := counter[int](subs)
			bus := pubsub.NewBus[int](append(tc.opts, hook)...)
			got := make([][]int, subs)
			dones := make([]<-chan error, subs)
			for i := range subs {
				dones[i] = subscribe(bus, ctx, "t", collectUntil(&got[i], n))
			}
			recv(t, ctx, ready)
			if err := bus.Publish(ctx, "t", seq(n)...); err != nil {
				t.Fatalf("Publish: %v", err)
			}
			for i := range subs {
				if err := recv(t, ctx, dones[i]); !errors.Is(err, errStop) {
					t.Fatalf("sub %d: %v", i, err)
				}
				if !slices.Equal(got[i], seq(n)) {
					t.Fatalf("sub %d got %v", i, got[i])
				}
			}
		})
	}
}

// Every subscriber sees one order, and each publisher's messages keep their order, with concurrent publishers.
func TestBus_sameOrderForEverySubscriber(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const subs, pubs, per = 3, 4, 50
	hook, ready := counter[[2]int](subs)
	bus := pubsub.NewBus[[2]int](pubsub.WithBuffer[[2]int](2), hook)
	got := make([][][2]int, subs)
	dones := make([]<-chan error, subs)
	for i := range subs {
		dones[i] = subscribe(bus, ctx, "t", collectUntil(&got[i], pubs*per))
	}
	recv(t, ctx, ready)
	var wg sync.WaitGroup
	for p := range pubs {
		wg.Go(func() {
			for i := range per {
				if err := bus.Publish(ctx, "t", [2]int{p, i}); err != nil {
					t.Errorf("Publish: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()
	for i := range subs {
		if err := recv(t, ctx, dones[i]); !errors.Is(err, errStop) {
			t.Fatalf("sub %d: %v", i, err)
		}
		if !slices.Equal(got[i], got[0]) {
			t.Fatalf("sub %d saw another order than sub 0", i)
		}
	}
	next := make([]int, pubs)
	for _, m := range got[0] {
		if m[1] != next[m[0]] {
			t.Fatalf("publisher %d: got %d, want %d (per-publisher order lost)", m[0], m[1], next[m[0]])
		}
		next[m[0]]++
	}
}

// A slow subscriber holds Publish back (rule 5): shown with channels and synctest, no sleep.
func TestBus_backpressurePublishBlocksOnSlowSubscriber(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		bus := pubsub.NewBus[int](pubsub.WithBuffer[int](1))
		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		var got []int
		done := subscribe(bus, ctx, "t", func(_ context.Context, n int) error {
			<-release
			got = append(got, n)
			if len(got) == 3 {
				cancel()
			}
			return nil
		})
		synctest.Wait()                                     // registered, waiting for a message
		if err := bus.Publish(ctx, "t", 1, 2); err != nil { // 1 taken by the handler, 2 buffered
			t.Fatalf("Publish: %v", err)
		}
		var returned atomic.Bool
		pubDone := make(chan error, 1)
		go func() { pubDone <- bus.Publish(ctx, "t", 3); returned.Store(true) }()
		synctest.Wait()
		if returned.Load() {
			t.Fatal("Publish returned while the subscriber was busy and its buffer full")
		}
		close(release)
		if err := <-pubDone; err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Subscribe: %v", err)
		}
		if !slices.Equal(got, []int{1, 2, 3}) {
			t.Fatalf("got %v", got)
		}
	})
}

// A busy subscriber holds Publish back for the rest of its messages, whatever the other subscribers do.
func TestBus_slowSubscriberHoldsPublishForOthers(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		bus := pubsub.NewBus[int]()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		slow := subscribe(bus, ctx, "t", func(context.Context, int) error { <-release; return errStop })
		synctest.Wait()
		var fastGot atomic.Int64
		fast := subscribe(bus, ctx, "t", func(context.Context, int) error { fastGot.Add(1); return errStop })
		synctest.Wait()
		pubDone := make(chan error, 1)
		go func() { pubDone <- bus.Publish(ctx, "t", 1, 2) }()
		synctest.Wait()
		select {
		case err := <-pubDone:
			t.Fatalf("Publish returned %v while the first subscriber held message 1", err)
		default:
		}
		close(release)
		if err := <-pubDone; err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if err := <-slow; !errors.Is(err, errStop) {
			t.Fatalf("slow: %v", err)
		}
		if err := <-fast; !errors.Is(err, errStop) || fastGot.Load() != 1 {
			t.Fatalf("fast: %v, %d calls", err, fastGot.Load())
		}
	})
}

func TestWithDropOldest_reportsExactlyWhatItDropped(t *testing.T) {
	leak.Verify(t)
	for _, tc := range []struct {
		buffer           int
		publish          []int
		dropped, gotTail []int
	}{
		{buffer: 0, publish: []int{2, 3, 4}, dropped: []int{2, 3}, gotTail: []int{4}}, // buffer 0 means 1
		{buffer: 1, publish: []int{2, 3, 4}, dropped: []int{2, 3}, gotTail: []int{4}},
		{buffer: 2, publish: []int{2, 3, 4, 5, 6}, dropped: []int{2, 3, 4}, gotTail: []int{5, 6}},
		{buffer: 3, publish: []int{2, 3}, dropped: nil, gotTail: []int{2, 3}},
	} {
		t.Run(fmt.Sprintf("buffer%d", tc.buffer), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				type drop struct {
					topic string
					n     int
				}
				var dropped []drop
				bus := pubsub.NewBus[int](pubsub.WithBuffer[int](tc.buffer),
					pubsub.WithDropOldest(func(topic string, n int) { dropped = append(dropped, drop{topic, n}) }))
				ctx, cancel := context.WithCancel(t.Context())
				release := make(chan struct{})
				want := append([]int{1}, tc.gotTail...)
				var got []int
				done := subscribe(bus, ctx, "ticks", func(_ context.Context, n int) error {
					got = append(got, n)
					if n == 1 {
						<-release
					}
					if len(got) == len(want) {
						cancel()
					}
					return nil
				})
				synctest.Wait()
				if err := bus.Publish(ctx, "ticks", 1); err != nil {
					t.Fatal(err)
				}
				synctest.Wait() // the handler holds 1
				if err := bus.Publish(ctx, "ticks", tc.publish...); err != nil {
					t.Fatalf("Publish never blocks with WithDropOldest: %v", err)
				}
				close(release)
				if err := <-done; !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "not delivered") {
					t.Fatalf("Subscribe: %v", err)
				}
				var gotDrops []int
				for _, d := range dropped {
					if d.topic != "ticks" {
						t.Fatalf("report topic %q", d.topic)
					}
					gotDrops = append(gotDrops, d.n)
				}
				if !slices.Equal(gotDrops, tc.dropped) || !slices.Equal(got, want) {
					t.Fatalf("dropped %v got %v, want dropped %v got %v", gotDrops, got, tc.dropped, want)
				}
				if len(got)+len(gotDrops) != 1+len(tc.publish) {
					t.Fatal("a message was neither delivered nor reported")
				}
			})
		})
	}
}

func TestWithDropOldest_reportPanicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		bus := pubsub.NewBus[int](pubsub.WithDropOldest(func(string, int) { panic("report") }))
		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})
		done := subscribe(bus, ctx, "t", func(context.Context, int) error { <-release; return nil })
		synctest.Wait()
		_ = bus.Publish(ctx, "t", 1)
		synctest.Wait()
		_ = bus.Publish(ctx, "t", 2) // buffered
		if err := bus.Publish(ctx, "t", 3); !errors.Is(err, panics.ErrPanic) {
			t.Fatalf("Publish: %v", err)
		}
		cancel()
		close(release)
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Subscribe: %v", err)
		}
	})
}

func TestBus_publishNoSubscribers(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var bus pubsub.Bus[int]
	if err := bus.Publish(ctx, "nobody"); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(ctx, "nobody", 1, 2, 3); err != nil {
		t.Fatal(err)
	}
	// A topic whose last subscriber left is empty again.
	hook, ready := counter[int](1)
	bus2 := pubsub.NewBus[int](hook)
	done := subscribe(bus2, ctx, "t", func(context.Context, int) error { return errStop })
	recv(t, ctx, ready)
	if err := bus2.Publish(ctx, "t", 1); err != nil {
		t.Fatal(err)
	}
	recv(t, ctx, done)
	if err := bus2.Publish(ctx, "t", 2); err != nil {
		t.Fatal(err)
	}
}

func TestBus_closed(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	bus := pubsub.NewBus[int]()
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := bus.Publish(ctx, "t", 1); !errors.Is(err, pubsub.ErrClosed) {
		t.Fatalf("Publish after Close: %v", err)
	}
	if err := bus.Publish(ctx, "t"); !errors.Is(err, pubsub.ErrClosed) {
		t.Fatalf("empty Publish after Close: %v", err)
	}
	if err := bus.Subscribe(ctx, "t", func(context.Context, int) error { return nil }); !errors.Is(err, pubsub.ErrClosed) {
		t.Fatalf("Subscribe after Close: %v", err)
	}
	var zero pubsub.Bus[int]
	if err := zero.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zero.Publish(ctx, "t", 1); !errors.Is(err, pubsub.ErrClosed) {
		t.Fatal(err)
	}
	var nilBus *pubsub.Bus[int]
	if nilBus.Close() != nil || !errors.Is(nilBus.Publish(ctx, "t", 1), pubsub.ErrClosed) ||
		!errors.Is(nilBus.Subscribe(ctx, "t", func(context.Context, int) error { return nil }), pubsub.ErrClosed) {
		t.Fatal("nil *Bus")
	}
}

func TestBus_invalid(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	h := func(context.Context, string) error { return nil }
	for name, bus := range map[string]*pubsub.Bus[string]{
		"bufferTooLarge": pubsub.NewBus[string](pubsub.WithBuffer[string](1<<20 + 1)),
	} {
		if err := bus.Publish(ctx, "t", "x"); !errors.Is(err, pubsub.ErrInvalid) {
			t.Fatalf("%s Publish: %v", name, err)
		}
		if err := bus.Subscribe(ctx, "t", h); !errors.Is(err, pubsub.ErrInvalid) {
			t.Fatalf("%s Subscribe: %v", name, err)
		}
	}
	if err := pubsub.NewBus[string]().Subscribe(ctx, "t", nil); !errors.Is(err, pubsub.ErrInvalid) {
		t.Fatalf("nil handler: %v", err)
	}
	// Valid edges: a nil Option, a negative buffer (0), a nil report, a nil hook, the maximum buffer.
	for _, bus := range []*pubsub.Bus[string]{
		pubsub.NewBus[string](nil, pubsub.WithBuffer[string](-5), pubsub.WithOnSubscribers[string](nil)),
		pubsub.NewBus[string](pubsub.WithDropOldest[string](nil)),
		pubsub.NewBus[string](pubsub.WithBuffer[string](1 << 20)),
	} {
		if err := bus.Publish(ctx, "t", "x"); err != nil {
			t.Fatalf("valid bus: %v", err)
		}
	}
}

func TestBus_nilContext(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	hook, ready := counter[int](1)
	bus := pubsub.NewBus[int](hook)
	done := make(chan error, 1)
	go func() {
		done <- bus.Subscribe(nil, "t", func(context.Context, int) error { return errStop }) // nil ctx: context.Background()
	}()
	recv(t, ctx, ready)
	if err := bus.Publish(nil, "t", 1); err != nil { // nil ctx: context.Background()
		t.Fatal(err)
	}
	if err := recv(t, ctx, done); !errors.Is(err, errStop) {
		t.Fatal(err)
	}
}

func TestBus_handlerErrorEndsSubscribe(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	hook, ready := counter[int](2)
	bus := pubsub.NewBus[int](hook)
	failing := subscribe(bus, ctx, "t", func(_ context.Context, n int) error {
		if n == 2 {
			return errA
		}
		return nil
	})
	var other []int
	healthy := subscribe(bus, ctx, "t", collectUntil(&other, 3))
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "t", 1, 2, 3); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, ctx, failing); !errors.Is(err, errA) {
		t.Fatalf("failing: %v", err)
	}
	if err := recv(t, ctx, healthy); !errors.Is(err, errStop) || !slices.Equal(other, []int{1, 2, 3}) {
		t.Fatalf("one subscriber's error must not touch another: %v %v", err, other)
	}
}

func TestBus_handlerPanicIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	hook, ready := counter[int](1)
	bus := pubsub.NewBus[int](hook)
	done := subscribe(bus, ctx, "t", func(context.Context, int) error { panic("boom") })
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "t", 1); err != nil {
		t.Fatal(err)
	}
	err := recv(t, ctx, done)
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("got %v", err)
	}
	// The bus keeps working.
	done = subscribe(bus, ctx, "t", func(context.Context, int) error { return errStop })
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "t", 2); err != nil {
		t.Fatal(err)
	}
	if err := recv(t, ctx, done); !errors.Is(err, errStop) {
		t.Fatal(err)
	}
}

func TestWithOnSubscribers_panic(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	bus := pubsub.NewBus[int](pubsub.WithOnSubscribers[int](func(string, int) { panic("hook") }))
	if err := bus.Subscribe(ctx, "t", func(context.Context, int) error { return nil }); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("got %v", err)
	}
	if err := bus.Publish(ctx, "t", 1); err != nil { // the failed subscriber is not registered
		t.Fatal(err)
	}
}

// Subscribe returns on cancel only after the running handler has returned, and the handler saw its ctx done.
func TestBus_subscribeReturnsOnCancelWithHandlerFinished(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		for _, viaClose := range []bool{false, true} {
			bus := pubsub.NewBus[int]()
			ctx, cancel := context.WithCancel(t.Context())
			inHandler, release := make(chan struct{}), make(chan struct{})
			var handlerCause error
			var finished atomic.Bool
			done := subscribe(bus, ctx, "t", func(hctx context.Context, _ int) error {
				close(inHandler)
				<-release
				<-hctx.Done()
				handlerCause = context.Cause(hctx)
				finished.Store(true)
				return nil
			})
			synctest.Wait()
			_ = bus.Publish(ctx, "t", 1)
			<-inHandler
			if viaClose {
				_ = bus.Close()
			} else {
				cancel()
			}
			synctest.Wait()
			select {
			case err := <-done:
				t.Fatalf("Subscribe returned %v before its handler did", err)
			default:
			}
			close(release)
			err := <-done
			if !finished.Load() {
				t.Fatal("handler not finished")
			}
			want := context.Canceled
			if viaClose {
				want = pubsub.ErrClosed
			}
			if !errors.Is(err, want) || !errors.Is(handlerCause, want) {
				t.Fatalf("close=%v: Subscribe %v, handler cause %v", viaClose, err, handlerCause)
			}
			cancel()
		}
	})
}

func TestBus_subscribeCancelBeforeStart(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancelCause(bounded(t))
	cancel(errA)
	var calls atomic.Int64
	bus := pubsub.NewBus[int]()
	if err := bus.Subscribe(ctx, "t", func(context.Context, int) error { calls.Add(1); return nil }); !errors.Is(err, errA) {
		t.Fatalf("got %v", err)
	}
	if err := bus.Publish(ctx, "t", 1); !errors.Is(err, errA) {
		t.Fatalf("Publish with a done ctx: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("handler ran")
	}
}

// Messages left in a subscriber's buffer when it leaves are counted in its error, never silent.
func TestBus_undeliveredCounted(t *testing.T) {
	leak.Verify(t)
	for _, drop := range []bool{false, true} {
		t.Run(fmt.Sprintf("dropOldest=%v", drop), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				opts := []pubsub.Option[int]{pubsub.WithBuffer[int](3)}
				if drop {
					opts = append(opts, pubsub.WithDropOldest(func(string, int) { t.Error("nothing should be dropped") }))
				}
				bus := pubsub.NewBus[int](opts...)
				ctx, cancel := context.WithCancel(t.Context())
				release := make(chan struct{})
				var calls atomic.Int64
				done := subscribe(bus, ctx, "t", func(context.Context, int) error { calls.Add(1); <-release; return nil })
				synctest.Wait()
				_ = bus.Publish(ctx, "t", 1)
				synctest.Wait() // the handler holds 1
				if err := bus.Publish(ctx, "t", 2, 3, 4); err != nil {
					t.Fatal(err)
				}
				cancel()
				close(release)
				err := <-done
				if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "pubsub: 3 buffered messages not delivered") {
					t.Fatalf("got %v", err)
				}
				if calls.Load() != 1 {
					t.Fatalf("handler ran %d times after cancel", calls.Load())
				}
			})
		})
	}
}

func TestBus_publishCancelWhileBlocked(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		bus := pubsub.NewBus[int]()
		subCtx, stopSub := context.WithCancel(t.Context())
		release := make(chan struct{})
		done := subscribe(bus, subCtx, "t", func(context.Context, int) error { <-release; return nil })
		synctest.Wait()
		_ = bus.Publish(t.Context(), "t", 1) // taken; the handler is now busy
		pubCtx, cancelPub := context.WithCancelCause(t.Context())
		pubDone := make(chan error, 1)
		go func() { pubDone <- bus.Publish(pubCtx, "t", 2) }()
		synctest.Wait()
		cancelPub(errA)
		if err := <-pubDone; !errors.Is(err, errA) {
			t.Fatalf("Publish: %v", err)
		}
		// A second publisher waiting for the topic's turn also returns on its ctx.
		pubDone2 := make(chan error, 1)
		go func() { pubDone2 <- bus.Publish(t.Context(), "t", 3) }()
		synctest.Wait()
		turnCtx, cancelTurn := context.WithCancel(t.Context())
		waitTurn := make(chan error, 1)
		go func() { waitTurn <- bus.Publish(turnCtx, "t", 4) }()
		synctest.Wait()
		cancelTurn()
		if err := <-waitTurn; !errors.Is(err, context.Canceled) {
			t.Fatalf("Publish waiting for its turn: %v", err)
		}
		stopSub()
		close(release)
		if err := <-pubDone2; err != nil { // the subscriber left: it is skipped
			t.Fatalf("Publish to a subscriber that left: %v", err)
		}
		<-done
	})
}

func TestBus_closeUnblocksPublish(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		bus := pubsub.NewBus[int]()
		release := make(chan struct{})
		done := subscribe(bus, t.Context(), "t", func(context.Context, int) error { <-release; return nil })
		synctest.Wait()
		_ = bus.Publish(t.Context(), "t", 1)
		blocked := make(chan error, 1)
		go func() { blocked <- bus.Publish(t.Context(), "t", 2) }()
		waiting := make(chan error, 1)
		synctest.Wait()
		go func() { waiting <- bus.Publish(t.Context(), "t", 3) }() // waits for the turn
		synctest.Wait()
		_ = bus.Close()
		if err := <-blocked; !errors.Is(err, pubsub.ErrClosed) {
			t.Fatalf("blocked Publish: %v", err)
		}
		if err := <-waiting; !errors.Is(err, pubsub.ErrClosed) {
			t.Fatalf("waiting Publish: %v", err)
		}
		close(release)
		if err := <-done; !errors.Is(err, pubsub.ErrClosed) {
			t.Fatalf("Subscribe: %v", err)
		}
	})
}

// A subscriber that leaves while Publish waits on it is skipped; the others still get the message.
func TestBus_leavingSubscriberIsSkipped(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		bus := pubsub.NewBus[int]()
		leaveCtx, leave := context.WithCancel(t.Context())
		release := make(chan struct{})
		leaving := subscribe(bus, leaveCtx, "t", func(context.Context, int) error { <-release; return nil })
		synctest.Wait()
		var got []int
		stays := subscribe(bus, t.Context(), "t", collectUntil(&got, 2))
		synctest.Wait()
		_ = bus.Publish(t.Context(), "t", 1) // the leaving subscriber now holds 1
		pubDone := make(chan error, 1)
		go func() { pubDone <- bus.Publish(t.Context(), "t", 2) }()
		synctest.Wait()
		leave()
		close(release)
		if err := <-pubDone; err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if err := <-leaving; !errors.Is(err, context.Canceled) {
			t.Fatalf("leaving: %v", err)
		}
		if err := <-stays; !errors.Is(err, errStop) || !slices.Equal(got, []int{1, 2}) {
			t.Fatalf("stays: %v %v", err, got)
		}
	})
}

func TestBus_manyTopics(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const topics, n = 20, 10
	hook, ready := counter[string](1)
	bus := pubsub.NewBus[string](pubsub.WithBuffer[string](1), hook)
	got := make([][]string, topics)
	dones := make([]<-chan error, topics)
	for i := range topics {
		dones[i] = subscribe(bus, ctx, fmt.Sprint("topic-", i), collectUntil(&got[i], n))
	}
	for range topics {
		recv(t, ctx, ready)
	}
	var wg sync.WaitGroup
	for i := range topics {
		wg.Go(func() {
			for j := range n {
				if err := bus.Publish(ctx, fmt.Sprint("topic-", i), fmt.Sprint(i, ":", j)); err != nil {
					t.Errorf("Publish: %v", err)
				}
			}
		})
	}
	wg.Wait()
	for i := range topics {
		if err := recv(t, ctx, dones[i]); !errors.Is(err, errStop) {
			t.Fatal(err)
		}
		for j, m := range got[i] {
			if m != fmt.Sprint(i, ":", j) {
				t.Fatalf("topic %d message %d = %q (crossed topics or reordered)", i, j, m)
			}
		}
	}
}

// go_44 worker: the consumer goroutine loops forever after the stop signal. Ours returns on cancel, and the
// leak check sees no goroutine left.
func TestSubscribe_defectGo44_consumerGoroutineExits(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	hook, ready := counter[int](5)
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](4), hook)
	subCtx, cancel := context.WithCancel(ctx)
	dones := make([]<-chan error, 5)
	for i := range dones {
		dones[i] = subscribe(bus, subCtx, "orders", func(context.Context, int) error { return nil })
	}
	recv(t, ctx, ready)
	cancel()
	for _, d := range dones {
		if err := recv(t, ctx, d); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
}

// go_44 producer opens a producer per request; go_58 shares one AMQP channel across goroutines unguarded.
// Ours: one Bus shared by many concurrent publishers and subscribers, race-free (run under -race).
func TestPublish_defectGo44Go58_oneBusSharedByConcurrentPublishers(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const pubs, per = 8, 100
	hook, ready := counter[int](1)
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](8), hook)
	var sum atomic.Int64
	var count int
	done := subscribe(bus, ctx, "orders", func(_ context.Context, n int) error {
		sum.Add(int64(n))
		count++
		if count == pubs*per {
			return errStop
		}
		return nil
	})
	recv(t, ctx, ready)
	var wg sync.WaitGroup
	for range pubs {
		wg.Go(func() {
			for i := range per {
				if err := bus.Publish(ctx, "orders", i); err != nil {
					t.Errorf("Publish: %v", err)
				}
			}
		})
	}
	wg.Wait()
	if err := recv(t, ctx, done); !errors.Is(err, errStop) {
		t.Fatal(err)
	}
	if want := int64(pubs * per * (per - 1) / 2); sum.Load() != want {
		t.Fatalf("sum %d, want %d (lost or duplicated messages)", sum.Load(), want)
	}
}

// go_58 consumer reads `<-messages` without checking ok, so once the channel closes it spins on the zero
// value. Ours: after Close the handler is never called again (no zero values) and Subscribe returns.
func TestSubscribe_defectGo58_noSpinAfterClose(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	hook, ready := counter[int](1)
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](2), hook)
	var calls, zeros atomic.Int64
	got := make(chan struct{}, 3)
	done := subscribe(bus, ctx, "q", func(_ context.Context, n int) error {
		calls.Add(1)
		if n == 0 {
			zeros.Add(1)
		}
		got <- struct{}{}
		return nil
	})
	recv(t, ctx, ready)
	if err := bus.Publish(ctx, "q", 1, 2, 3); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		recv(t, ctx, got)
	}
	_ = bus.Close()
	if err := recv(t, ctx, done); !errors.Is(err, pubsub.ErrClosed) {
		t.Fatal(err)
	}
	if calls.Load() != 3 || zeros.Load() != 0 {
		t.Fatalf("handler calls %d, zero values %d", calls.Load(), zeros.Load())
	}
}

// go_58 consumes with auto-ack, so a message the consumer has not processed is lost without a trace. Ours is
// at-most-once too, but the loss is counted in Subscribe's error (TestBus_undeliveredCounted) and a handler
// error is returned, never acknowledged silently.
func TestSubscribe_defectGo58_failedMessageNotSilentlyAcked(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	hook, ready := counter[int](1)
	bus := pubsub.NewBus[int](hook)
	done := subscribe(bus, ctx, "q", func(_ context.Context, n int) error { return fmt.Errorf("message %d: %w", n, errA) })
	recv(t, ctx, ready)
	_ = bus.Publish(ctx, "q", 7)
	if err := recv(t, ctx, done); !errors.Is(err, errA) || !strings.Contains(err.Error(), "message 7") {
		t.Fatalf("got %v", err)
	}
}

func BenchmarkBus_Publish(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 1)
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](64), pubsub.WithOnSubscribers[int](func(_ string, n int) {
		if n == 1 {
			ready <- struct{}{}
		}
	}))
	done := make(chan error, 1)
	go func() { done <- bus.Subscribe(ctx, "t", func(context.Context, int) error { return nil }) }()
	<-ready
	b.ReportAllocs()
	for b.Loop() {
		if err := bus.Publish(ctx, "t", 1); err != nil {
			b.Fatal(err)
		}
	}
	cancel()
	<-done
}

// A subscriber registering while a Publish is in progress gets none of that Publish's messages (the subscriber
// list is the snapshot taken when the Publish got the topic's turn) and all of the next Publish's.
func TestBus_subscribeDuringPublishSeesSnapshot(t *testing.T) {
	leak.Verify(t)
	synctest.Test(t, func(t *testing.T) {
		bus := pubsub.NewBus[int](pubsub.WithBuffer[int](1))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		var gotA []int
		a := subscribe(bus, ctx, "t", func(_ context.Context, n int) error {
			if n == 1 {
				<-release
			}
			gotA = append(gotA, n)
			if len(gotA) == 5 {
				return errStop
			}
			return nil
		})
		synctest.Wait()
		pubDone := make(chan error, 1)
		go func() { pubDone <- bus.Publish(ctx, "t", 1, 2, 3) }()
		synctest.Wait() // 1 held by a's handler, 2 buffered, the Publish blocked on 3
		select {
		case err := <-pubDone:
			t.Fatalf("Publish returned %v while a held message 1", err)
		default:
		}
		var bCalls atomic.Int64
		var gotB []int
		b := subscribe(bus, ctx, "t", func(_ context.Context, n int) error {
			bCalls.Add(1)
			gotB = append(gotB, n)
			if len(gotB) == 2 {
				return errStop
			}
			return nil
		})
		synctest.Wait() // b is registered during the Publish
		close(release)
		if err := <-pubDone; err != nil {
			t.Fatalf("Publish: %v", err)
		}
		synctest.Wait()
		if bCalls.Load() != 0 {
			t.Fatalf("a subscriber registered during a Publish got %d of its messages", bCalls.Load())
		}
		if err := bus.Publish(ctx, "t", 4, 5); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if err := <-a; !errors.Is(err, errStop) || !slices.Equal(gotA, []int{1, 2, 3, 4, 5}) {
			t.Fatalf("a: %v %v", err, gotA)
		}
		if err := <-b; !errors.Is(err, errStop) || !slices.Equal(gotB, []int{4, 5}) {
			t.Fatalf("b: %v %v", err, gotB)
		}
	})
}

// The per-call cost of the topic lookup under Bus.mu without contention: 64 topics, one draining subscriber
// each, ONE publisher round-robin. Compare with BenchmarkBus_Publish (one topic) apples to apples.
func BenchmarkBus_Publish_manyTopicsSerial(b *testing.B) {
	const topics = 64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, topics)
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](64), pubsub.WithOnSubscribers[int](func(_ string, n int) {
		if n == 1 {
			ready <- struct{}{}
		}
	}))
	names := make([]string, topics)
	dones := make([]chan error, topics)
	for i := range topics {
		names[i] = fmt.Sprint("topic-", i)
		dones[i] = make(chan error, 1)
		go func() { dones[i] <- bus.Subscribe(ctx, names[i], func(context.Context, int) error { return nil }) }()
	}
	for range topics {
		<-ready
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if err := bus.Publish(ctx, names[i%topics], 1); err != nil {
			b.Fatal(err)
		}
		i++
	}
	cancel()
	for _, d := range dones {
		<-d
	}
}

// Total throughput with 64 topics and publishers in parallel (MATRIX §6). Its ns/op is wall time over all
// goroutines: it does NOT isolate Bus.mu; read it beside BenchmarkBus_Publish_manyTopicsSerial.
func BenchmarkBus_Publish_manyTopicsManyPublishers(b *testing.B) {
	const topics = 64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, topics)
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](64), pubsub.WithOnSubscribers[int](func(_ string, n int) {
		if n == 1 {
			ready <- struct{}{}
		}
	}))
	names := make([]string, topics)
	dones := make([]chan error, topics)
	for i := range topics {
		names[i] = fmt.Sprint("topic-", i)
		dones[i] = make(chan error, 1)
		go func() { dones[i] <- bus.Subscribe(ctx, names[i], func(context.Context, int) error { return nil }) }()
	}
	for range topics {
		<-ready
	}
	var next atomic.Int64
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		topic := names[int(next.Add(1)-1)%topics]
		for pb.Next() {
			if err := bus.Publish(ctx, topic, 1); err != nil {
				b.Error(err)
				return
			}
		}
	})
	cancel()
	for _, d := range dones {
		<-d
	}
}
