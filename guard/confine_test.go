package guard_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/guard"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func ExampleConfine() {
	reqs := make(chan func(*int))
	type result struct {
		n   int
		err error
	}
	done := make(chan result)
	go func() { // the owner: the only goroutine that touches the counter
		n, err := guard.Confine(context.Background(), 0, reqs)
		done <- result{n, err}
	}()

	for range 3 {
		reqs <- func(n *int) { *n++ }
	}
	reply := make(chan int)
	reqs <- func(n *int) { reply <- *n } // a request with an answer closes over its own reply channel
	fmt.Println("so far", <-reply)

	close(reqs) // stops the owner once every request sent has run
	r := <-done
	fmt.Println("final", r.n, r.err)
	// Output:
	// so far 3
	// final 3 <nil>
}

// runConfine starts Confine on its own goroutine and returns a function that waits, bounded by ctx, for its
// result.
func runConfine[S any](t *testing.T, ctx context.Context, state S, reqs <-chan func(*S)) func() (S, error) {
	t.Helper()
	type result struct {
		s   S
		err error
	}
	done := make(chan result, 1)
	go func() {
		s, err := guard.Confine(ctx, state, reqs)
		done <- result{s, err}
	}()
	return func() (S, error) {
		t.Helper()
		wait, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		select {
		case r := <-done:
			return r.s, r.err
		case <-wait.Done():
			t.Fatal("Confine did not return")
			var zero S
			return zero, nil
		}
	}
}

func TestConfine_reqsClosed(t *testing.T) {
	reqs := make(chan func(*[]int), 3)
	for i := range 3 {
		reqs <- func(s *[]int) { *s = append(*s, i) }
	}
	close(reqs)
	got, err := guard.Confine(t.Context(), nil, reqs)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if fmt.Sprint(got) != "[0 1 2]" {
		t.Fatalf("state = %v, want [0 1 2] (every request, in arrival order)", got)
	}
}

func TestConfine_nilReqs(t *testing.T) {
	got, err := guard.Confine[int](t.Context(), 5, nil)
	if got != 5 || err != nil {
		t.Fatalf("Confine(nil reqs) = %d, %v; want 5, nil at once", got, err)
	}
}

func TestConfine_nilCtx(t *testing.T) {
	reqs := make(chan func(*int), 1)
	reqs <- func(n *int) { *n = 1 }
	close(reqs)
	//lint:ignore SA1012 a nil ctx is documented as context.Background()
	got, err := guard.Confine(nil, 0, reqs) //nolint:staticcheck
	if got != 1 || err != nil {
		t.Fatalf("Confine(nil ctx) = %d, %v; want 1, nil", got, err)
	}
}

func TestConfine_nilRequest(t *testing.T) {
	reqs := make(chan func(*int), 3)
	reqs <- func(n *int) { *n++ }
	reqs <- nil
	reqs <- func(n *int) { *n++ }
	close(reqs)
	got, err := guard.Confine(t.Context(), 0, reqs)
	if got != 2 || err != nil {
		t.Fatalf("Confine = %d, %v; want 2, nil (nil request skipped)", got, err)
	}
}

func TestConfine_cancelBefore(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	reqs := make(chan func(*int), 1)
	reqs <- func(n *int) { *n = 99 }
	got, err := guard.Confine(ctx, 0, reqs)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got != 0 {
		t.Fatalf("state = %d, want 0 (no request runs after ctx is done)", got)
	}
}

func TestConfine_cancelWhileWaiting(t *testing.T) {
	leak.Verify(t)
	cause := errors.New("shutting down")
	ctx, cancel := context.WithCancelCause(t.Context())
	reqs := make(chan func(*int)) // never closed, never sent on: Confine waits
	served := make(chan struct{})
	wait := runConfine(t, ctx, 0, reqs)
	reqs <- func(n *int) { *n = 4; close(served) }
	<-served
	cancel(cause)
	got, err := wait()
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the cancel cause", err)
	}
	if got != 4 {
		t.Fatalf("state = %d, want 4", got)
	}
}

func TestConfine_cancelDuringRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reqs := make(chan func(*int), 2)
	reqs <- func(n *int) { cancel(); *n = 1 } // the running request finishes
	reqs <- func(n *int) { *n = 2 }           // received after ctx is done: not run
	got, err := guard.Confine(ctx, 0, reqs)
	if !errors.Is(err, context.Canceled) || got != 1 {
		t.Fatalf("Confine = %d, %v; want 1, context.Canceled", got, err)
	}
}

func TestConfine_panic(t *testing.T) {
	reqs := make(chan func(*int), 3)
	reqs <- func(n *int) { *n++ }
	reqs <- func(n *int) { *n++; panic("boom") }
	reqs <- func(n *int) { *n = 100 }
	got, err := guard.Confine(t.Context(), 0, reqs)
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err = %v, want panics.ErrPanic", err)
	}
	var pe *panics.PanicError
	if !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("err = %#v, want *panics.PanicError with Value \"boom\"", err)
	}
	if got != 2 {
		t.Fatalf("state = %d, want 2 (as the panic left it; later requests not run)", got)
	}
}

// TestConfine_receivedRequestRuns is the reviewer's regression: the sender cancels ctx the moment its unbuffered
// send completes (the owner has received the request). The request must still run and reply; before the fix it
// was dropped in that window and the sender waited on its reply forever.
func TestConfine_receivedRequestRuns(t *testing.T) {
	leak.Verify(t)
	for range 200 {
		ctx, cancel := context.WithCancel(t.Context())
		reqs := make(chan func(*int))
		wait := runConfine(t, ctx, 0, reqs)
		reply := make(chan int, 1)
		reqs <- func(n *int) { *n++; reply <- *n }
		cancel() // the owner has received the request: it may be anywhere between receive and run
		bound, stop := context.WithTimeout(t.Context(), 10*time.Second)
		select {
		case got := <-reply:
			if got != 1 {
				t.Fatalf("reply = %d, want 1", got)
			}
		case <-bound.Done():
			stop()
			t.Fatal("received request was dropped: no reply")
		}
		stop()
		got, err := wait()
		if got != 1 || !errors.Is(err, context.Canceled) {
			t.Fatalf("Confine = %d, %v; want 1, context.Canceled", got, err)
		}
	}
}

// TestConfine_goexit: runtime.Goexit in a request is not a panic; it ends the owner goroutine and Confine does
// not return (as panics.Try).
func TestConfine_goexit(t *testing.T) {
	leak.Verify(t)
	reqs := make(chan func(*int), 1)
	reqs <- func(*int) { runtime.Goexit() }
	close(reqs)
	returned := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = guard.Confine(t.Context(), 0, reqs)
		returned = true
	}()
	<-done
	if returned {
		t.Fatal("Confine returned after runtime.Goexit in a request")
	}
}

// TestConfine_go20OwnerNeverExits is the go_20 confinement defect: the owner loop had no return, so its
// goroutine lived forever. Ours returns when ctx is done and when reqs is closed, gives back the final state,
// and the goroutine count returns to the baseline (leak.Verify). 200 buyers race for 500 tickets under -race.
func TestConfine_go20OwnerNeverExits(t *testing.T) {
	buy := func(t *testing.T, reqs chan<- func(*int)) {
		var wg sync.WaitGroup
		for range 200 {
			wg.Go(func() {
				reqs <- func(tickets *int) {
					if *tickets > 0 {
						*tickets--
					}
				}
			})
		}
		wg.Wait()
	}
	t.Run("stoppedByCancel", func(t *testing.T) {
		leak.Verify(t)
		ctx, cancel := context.WithCancel(t.Context())
		reqs := make(chan func(*int))
		wait := runConfine(t, ctx, 500, reqs)
		buy(t, reqs)
		cancel()
		got, err := wait()
		if !errors.Is(err, context.Canceled) || got != 300 {
			t.Fatalf("Confine = %d, %v; want 300, context.Canceled", got, err)
		}
	})
	t.Run("stoppedByClose", func(t *testing.T) {
		leak.Verify(t)
		reqs := make(chan func(*int))
		wait := runConfine(t, t.Context(), 500, reqs)
		buy(t, reqs)
		close(reqs)
		got, err := wait()
		if err != nil || got != 300 {
			t.Fatalf("Confine = %d, %v; want 300, nil", got, err)
		}
	})
}

func BenchmarkConfine(b *testing.B) {
	b.ReportAllocs()
	reqs := make(chan func(*int))
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = guard.Confine(context.Background(), 0, reqs)
	}()
	inc := func(n *int) { *n++ }
	for b.Loop() {
		reqs <- inc
	}
	close(reqs)
	<-done
}
