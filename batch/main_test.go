package batch_test

import (
	"context"
	"iter"
	"os"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// testCtx bounds a test that waits (go-testing.md: at most 10 s).
func testCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 10*time.Second)
}

// upTo yields 1 .. n with nil errors.
func upTo(n int) iter.Seq2[int, error] {
	return func(yield func(int, error) bool) {
		for i := 1; i <= n; i++ {
			if !yield(i, nil) {
				return
			}
		}
	}
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

// recv receives one value from ch; it fails the test if ch is closed or bound ends first.
func recv[T any](t *testing.T, bound context.Context, ch <-chan T) T {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed, want a value")
		}
		return v
	case <-bound.Done():
		t.Fatal("no value before the test bound")
	}
	var zero T
	return zero
}

// send sends v on ch; it fails the test if bound ends first.
func send[T any](t *testing.T, bound context.Context, ch chan<- T, v T) {
	t.Helper()
	select {
	case ch <- v:
	case <-bound.Done():
		t.Fatalf("send of %v not received before the test bound", v)
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

// seqOf returns 1 .. n as a slice.
func seqOf(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i + 1
	}
	return s
}
