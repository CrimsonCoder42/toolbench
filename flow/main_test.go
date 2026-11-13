package flow_test

import (
	"context"
	"iter"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// testCtx bounds a test that waits (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

// collect receives from ch until it is closed; it fails the test if bound ends first.
func collect[T any](t *testing.T, bound context.Context, ch <-chan T) []T {
	t.Helper()
	var got []T
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, v)
		case <-bound.Done():
			t.Fatalf("channel not closed before the test bound: got %d values so far", len(got))
			return got
		}
	}
}

// filled returns a buffered channel holding vs, closed.
func filled[T any](vs ...T) <-chan T {
	ch := make(chan T, len(vs))
	for _, v := range vs {
		ch <- v
	}
	close(ch)
	return ch
}

// filledOpen returns a buffered channel holding vs, left open.
func filledOpen[T any](vs ...T) chan T {
	ch := make(chan T, len(vs))
	for _, v := range vs {
		ch <- v
	}
	return ch
}

// count yields 0, 1, 2, ... until the consumer stops.
func count() iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := 0; ; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

// upTo yields 0 .. n-1.
func upTo(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

// source is flow.FromSeq for tests whose subject is another export: the generator's wait func is not needed
// there (seq never panics; its goroutine ends with ctx or seq, which leak.Verify checks).
func source[T any](ctx context.Context, seq iter.Seq[T], opts ...flow.Option) <-chan T {
	out, _ := flow.FromSeq(ctx, seq, opts...)
	return out
}
