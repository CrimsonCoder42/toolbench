package pubsub_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/pubsub"
)

// publishNonBlocking runs Publish on its own goroutine and fails the test if it has not returned once every
// goroutine in the bubble is blocked (synctest: a fact, not a timing). Returns Publish's error.
func publishNonBlocking(t *testing.T, bus *pubsub.Bus[int], ctx context.Context, topic string, msgs ...int) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- bus.Publish(ctx, topic, msgs...) }()
	synctest.Wait()
	select {
	case err := <-done:
		return err
	default:
		t.Fatalf("Publish(%v) blocked on a subscriber", msgs)
		return nil
	}
}

// The slow subscriber is evicted with ErrSlowSubscriber (its handler ctx's cause, its Subscribe result with the
// undelivered count); the other subscriber gets every message; the publisher never blocks; the evicted
// subscriber gets nothing more. Buffer 0 keeps 1, as WithDropOldest does.
func TestWithDropSubscriber_evictsSlowSubscriberOthersGetEverything(t *testing.T) {
	leak.Verify(t)
	for _, buffer := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("buffer%d", buffer), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				keep := max(buffer, 1)
				total := keep + 3 // 1 held, keep buffered, 1 evicts, 1 skipped
				type rep struct {
					topic string
					n     int
				}
				var reported []rep
				bus := pubsub.NewBus[int](pubsub.WithBuffer[int](buffer),
					pubsub.WithDropSubscriber(func(topic string, n int) { reported = append(reported, rep{topic, n}) }))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				release := make(chan struct{})
				var slowCalls atomic.Int64
				var slowCause error
				slow := subscribe(bus, ctx, "t", func(hctx context.Context, _ int) error {
					slowCalls.Add(1)
					<-release
					<-hctx.Done()
					slowCause = context.Cause(hctx)
					return nil
				})
				synctest.Wait()
				var fastGot []int
				fast := subscribe(bus, ctx, "t", collectUntil(&fastGot, total))
				synctest.Wait()
				for i := 1; i <= total; i++ {
					if err := publishNonBlocking(t, bus, ctx, "t", i); err != nil {
						t.Fatalf("Publish(%d): %v", i, err)
					}
				}
				if err := <-fast; !errors.Is(err, errStop) || !slices.Equal(fastGot, seq(total)) {
					t.Fatalf("fast: %v %v", err, fastGot)
				}
				close(release)
				err := <-slow
				if !errors.Is(err, pubsub.ErrSlowSubscriber) || !strings.Contains(err.Error(), fmt.Sprintf("pubsub: %d buffered messages not delivered", keep)) {
					t.Fatalf("slow: %v", err)
				}
				if !errors.Is(slowCause, pubsub.ErrSlowSubscriber) {
					t.Fatalf("handler ctx cause: %v", slowCause)
				}
				if slowCalls.Load() != 1 {
					t.Fatalf("the evicted handler ran %d times", slowCalls.Load())
				}
				if !slices.Equal(reported, []rep{{"t", keep + 2}}) {
					t.Fatalf("reported %v, want only message %d", reported, keep+2)
				}
			})
		})
	}
}

// A panic in report is Publish's ErrPanic and the eviction has still happened; a nil report evicts silently.
func TestWithDropSubscriber_reportPanicAndNilReport(t *testing.T) {
	leak.Verify(t)
	for _, tc := range []struct {
		name    string
		report  func(string, int)
		wantPub error // nil: Publish returns nil
	}{
		{"nilReport", nil, nil},
		{"reportPanics", func(string, int) { panic("report") }, panics.ErrPanic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				bus := pubsub.NewBus[int](pubsub.WithDropSubscriber(tc.report))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				release := make(chan struct{})
				done := subscribe(bus, ctx, "t", func(context.Context, int) error { <-release; return nil })
				synctest.Wait()
				if err := publishNonBlocking(t, bus, ctx, "t", 1); err != nil { // held by the handler
					t.Fatal(err)
				}
				if err := publishNonBlocking(t, bus, ctx, "t", 2); err != nil { // buffered (buffer 0 keeps 1)
					t.Fatal(err)
				}
				err := publishNonBlocking(t, bus, ctx, "t", 3) // full: evicted
				if tc.wantPub == nil && err != nil || tc.wantPub != nil && !errors.Is(err, tc.wantPub) {
					t.Fatalf("Publish(3): %v, want %v", err, tc.wantPub)
				}
				if err := publishNonBlocking(t, bus, ctx, "t", 4); err != nil { // skipped, not reported again
					t.Fatalf("Publish(4) after eviction: %v", err)
				}
				close(release)
				if err := <-done; !errors.Is(err, pubsub.ErrSlowSubscriber) {
					t.Fatalf("Subscribe: %v", err)
				}
				// Close after the eviction: the Bus is still consistent.
				if err := bus.Close(); err != nil {
					t.Fatal(err)
				}
				if err := bus.Publish(ctx, "t", 5); !errors.Is(err, pubsub.ErrClosed) {
					t.Fatalf("Publish after Close: %v", err)
				}
			})
		})
	}
}

// The three slow-subscriber policies are mutually exclusive: the last drop option passed to NewBus wins.
func TestWithDropSubscriber_lastDropOptionWins(t *testing.T) {
	leak.Verify(t)
	t.Run("dropSubscriberLast", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var dropped, evicted []int
			bus := pubsub.NewBus[int](pubsub.WithBuffer[int](1),
				pubsub.WithDropOldest(func(_ string, n int) { dropped = append(dropped, n) }),
				pubsub.WithDropSubscriber(func(_ string, n int) { evicted = append(evicted, n) }))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			release := make(chan struct{})
			done := subscribe(bus, ctx, "t", func(context.Context, int) error { <-release; return nil })
			synctest.Wait()
			for i := 1; i <= 3; i++ {
				if err := publishNonBlocking(t, bus, ctx, "t", i); err != nil {
					t.Fatal(err)
				}
			}
			_ = bus.Close() // after the eviction: the first cause stays the cause
			close(release)
			err := <-done
			if !errors.Is(err, pubsub.ErrSlowSubscriber) || errors.Is(err, pubsub.ErrClosed) {
				t.Fatalf("Subscribe: %v", err)
			}
			if len(dropped) != 0 || !slices.Equal(evicted, []int{3}) {
				t.Fatalf("dropped %v evicted %v", dropped, evicted)
			}
		})
	})
	t.Run("dropOldestLast", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var dropped, evicted, got []int
			bus := pubsub.NewBus[int](pubsub.WithBuffer[int](1),
				pubsub.WithDropSubscriber(func(_ string, n int) { evicted = append(evicted, n) }),
				pubsub.WithDropOldest(func(_ string, n int) { dropped = append(dropped, n) }))
			ctx, cancel := context.WithCancel(t.Context())
			release := make(chan struct{})
			done := subscribe(bus, ctx, "t", func(_ context.Context, n int) error {
				got = append(got, n)
				if n == 1 {
					<-release
				}
				if len(got) == 2 {
					cancel()
				}
				return nil
			})
			synctest.Wait()
			for i := 1; i <= 3; i++ {
				if err := publishNonBlocking(t, bus, ctx, "t", i); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
			}
			close(release)
			err := <-done
			if !errors.Is(err, context.Canceled) || errors.Is(err, pubsub.ErrSlowSubscriber) {
				t.Fatalf("Subscribe: %v", err)
			}
			if !slices.Equal(dropped, []int{2}) || len(evicted) != 0 || !slices.Equal(got, []int{1, 3}) {
				t.Fatalf("dropped %v evicted %v got %v", dropped, evicted, got)
			}
		})
	})
}

// LLM-Projects hard-resume-agent/hub.go:43-48 and Videocall-Chat-GO-Project pkg/chat/hub.go:31-36 close the
// slow client's `send` channel from the hub while the client goroutine may still be sending on it. Ours never
// closes a subscriber's channel: with 4 concurrent publishers and 3 slow subscribers, every Publish during and
// after the evictions returns nil, nothing panics, each slow subscriber is evicted exactly once (the per-topic
// turn serialises publishers) and leaves with ErrSlowSubscriber.
func TestWithDropSubscriber_defectHub_closeFromTheHubRace(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const pubs, per, slows = 4, 50, 3
	hook, ready := counter[int](slows)
	var evicted atomic.Int64
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](1), hook,
		pubsub.WithDropSubscriber(func(string, int) { evicted.Add(1) }))
	release := make(chan struct{})
	dones := make([]<-chan error, slows)
	for i := range dones {
		dones[i] = subscribe(bus, ctx, "t", func(context.Context, int) error { <-release; return nil })
	}
	recv(t, ctx, ready)
	var wg sync.WaitGroup
	for range pubs {
		wg.Go(func() {
			for i := range per {
				if err := bus.Publish(ctx, "t", i); err != nil {
					t.Errorf("Publish: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()
	close(release)
	for i, d := range dones {
		if err := recv(t, ctx, d); !errors.Is(err, pubsub.ErrSlowSubscriber) {
			t.Fatalf("subscriber %d: %v", i, err)
		}
	}
	if evicted.Load() != slows {
		t.Fatalf("%d evictions reported, want %d", evicted.Load(), slows)
	}
}

// Every iteration evicts: a fresh subscriber whose handler waits on its ctx, buffer 1, then three Publish calls
// (held, buffered, evicting); the iteration ends when the evicted Subscribe has returned and unregistered. ns/op
// is that whole cycle; the evictions metric equals the iteration count.
func BenchmarkBus_Publish_dropSubscriber(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 1)
	var evictions atomic.Int64
	bus := pubsub.NewBus[int](pubsub.WithBuffer[int](1),
		pubsub.WithDropSubscriber(func(string, int) { evictions.Add(1) }),
		pubsub.WithOnSubscribers[int](func(_ string, n int) {
			if n == 1 {
				ready <- struct{}{}
			}
		}))
	done := make(chan error, 1)
	hold := func(hctx context.Context, _ int) error { <-hctx.Done(); return nil } // released by the eviction
	b.ReportAllocs()
	for b.Loop() {
		go func() { done <- bus.Subscribe(ctx, "t", hold) }()
		<-ready
		for i := 1; i <= 3; i++ { // 1 held or buffered, 2 buffered, the third finds the buffer full: evicted
			if err := bus.Publish(ctx, "t", i); err != nil {
				b.Fatal(err)
			}
		}
		if err := <-done; !errors.Is(err, pubsub.ErrSlowSubscriber) {
			b.Fatalf("Subscribe: %v", err)
		}
	}
	b.ReportMetric(float64(evictions.Load()), "evictions")
}
