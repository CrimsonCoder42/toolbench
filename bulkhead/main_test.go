package bulkhead

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

var (
	start   = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	errBoom = errors.New("boom")
)

// testCtx bounds a test (go-testing.md: at most 10 s).
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// events is a WithOnEvent hook that hands every Event to the test through a buffered channel (no test makes
// more calls than the buffer holds, so the hook never blocks).
type events struct{ ch chan Event }

func newEvents() *events { return &events{ch: make(chan Event, 256)} }

func (e *events) hook(ev Event) { e.ch <- ev }

// next returns the next event, failing the test if none arrives before ctx ends.
func (e *events) next(t *testing.T, ctx context.Context) Event {
	t.Helper()
	select {
	case ev := <-e.ch:
		return ev
	case <-ctx.Done():
		t.Fatalf("no event: %v", context.Cause(ctx))
		return Event{}
	}
}

// expect reads len(kinds) events and checks they are kinds, in order.
func (e *events) expect(t *testing.T, ctx context.Context, kinds ...EventKind) {
	t.Helper()
	for i, k := range kinds {
		if got := e.next(t, ctx).Kind; got != k {
			t.Fatalf("event %d = %v, want %v", i, got, k)
		}
	}
}

// expectAnyOrder reads len(kinds) events and checks they are kinds as a multiset (calls on different
// goroutines race to emit them).
func (e *events) expectAnyOrder(t *testing.T, ctx context.Context, kinds ...EventKind) {
	t.Helper()
	got := make([]EventKind, len(kinds))
	for i := range kinds {
		got[i] = e.next(t, ctx).Kind
	}
	slices.Sort(got)
	want := slices.Clone(kinds)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v in any order", got, want)
	}
}

// none checks that no event is pending.
func (e *events) none(t *testing.T) {
	t.Helper()
	select {
	case ev := <-e.ch:
		t.Fatalf("unexpected event %v", ev.Kind)
	default:
	}
}

// holder is a call that holds a slot until release is called.
type holder struct {
	release func()
	done    <-chan error
}

// hold starts a call on b whose fn blocks until release (or ctx ends) and returns once fn is running.
func hold(t *testing.T, ctx context.Context, b *Bulkhead) holder {
	t.Helper()
	entered, rel, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- b.Do(ctx, func(ctx context.Context) error {
			close(entered)
			select {
			case <-rel:
				return nil
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatalf("holder not admitted: %v", context.Cause(ctx))
	}
	return holder{release: func() { close(rel) }, done: done}
}

// call starts b.Do(ctx, fn) on a new goroutine and returns its result channel.
func call(ctx context.Context, b *Bulkhead, fn func(context.Context) error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- b.Do(ctx, fn) }()
	return done
}

// result waits for a call's result.
func result(t *testing.T, ctx context.Context, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatalf("call did not return: %v", context.Cause(ctx))
		return nil
	}
}

// assertIdle checks that every slot and queue place of b is free again.
func assertIdle(t *testing.T, b *Bulkhead) {
	t.Helper()
	if !b.slots.TryAcquire(int64(b.maxConc)) {
		t.Fatalf("a slot is still held")
	}
	b.slots.Release(int64(b.maxConc))
	if n := int64(b.maxConc) + int64(b.queue); !b.room.TryAcquire(n) {
		t.Fatalf("a queue place is still held")
	} else {
		b.room.Release(n)
	}
}

func ok(context.Context) error { return nil }

func mustNew(t *testing.T, maxConcurrent int, opts ...Option) *Bulkhead {
	t.Helper()
	b, err := New(maxConcurrent, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}
