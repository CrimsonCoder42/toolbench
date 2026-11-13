package group_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/CrimsonCoder42/toolbench/group"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

var errC = errors.New("c failed")

// unwrapAll returns the members of an errors.Join result, or the error alone when it is not a join.
func unwrapAll(err error) []error {
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		return u.Unwrap()
	}
	if err == nil {
		return nil
	}
	return []error{err}
}

func ExampleGroup_SetCollect() {
	var g group.Group
	g.SetCollect(true) // Wait returns every error, not only the first
	g.SetLimit(1)      // one at a time, so the completion order is the submission order
	g.Go(func() error { return errors.New("a failed") })
	g.Go(func() error { return errors.New("b failed") })
	g.Go(func() error { return nil })
	fmt.Println(g.Wait())
	// Output:
	// a failed
	// b failed
}

// Three failures: Wait returns a join that errors.Is matches for each, and the ctx was cancelled by the first.
func TestGroup_collectJoinsEveryError(t *testing.T) {
	leak.Verify(t)
	tctx := bounded(t)
	g, ctx := group.WithContext(tctx)
	g.SetCollect(true)
	g.Go(func() error { return errA })
	for _, e := range []error{errB, errC} {
		g.Go(func() error {
			select {
			case <-ctx.Done(): // after errA cancelled the ctx
				return e
			case <-tctx.Done():
				return tctx.Err()
			}
		})
	}
	err := g.Wait()
	for _, want := range []error{errA, errB, errC} {
		if !errors.Is(err, want) {
			t.Fatalf("Wait = %v, want it to match %v", err, want)
		}
	}
	if got := unwrapAll(err); len(got) != 3 || got[0] != errA {
		t.Fatalf("Unwrap = %v, want three errors with %v first", got, errA)
	}
	if cause := context.Cause(ctx); !errors.Is(cause, errA) || errors.Is(cause, errB) {
		t.Fatalf("cause = %v, want exactly the first error %v", cause, errA)
	}
}

func TestGroup_collectIncludesPanics(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetCollect(true)
	g.SetLimit(1)
	g.Go(func() error { return errA })
	g.Go(func() error { panic("boom") })
	g.Go(nil)
	err := g.Wait()
	if !errors.Is(err, errA) || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Wait = %v, want a join of %v and ErrPanic", err, errA)
	}
	var pe *panics.PanicError
	if !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("Wait = %v, want the PanicError with value boom reachable by errors.As", err)
	}
	if got := unwrapAll(err); len(got) != 3 {
		t.Fatalf("Unwrap = %d errors, want 3 (errA, the panic, the nil fn)", len(got))
	}
}

func TestGroup_collectIncludesErrZeroLimit(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetCollect(true)
	g.Go(func() error { return errA })
	_ = g.Wait()
	g.SetLimit(0)
	g.Go(func() error { return nil }) // not started: ErrZeroLimit is recorded
	err := g.Wait()
	if !errors.Is(err, errA) || !errors.Is(err, group.ErrZeroLimit) {
		t.Fatalf("Wait = %v, want a join of %v and ErrZeroLimit", err, errA)
	}
}

func TestGroup_collectNoErrorIsNil(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetCollect(true)
	for range 4 {
		g.Go(func() error { return nil })
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("Wait = %v, want nil", err)
	}
	if err := g.Wait(); err != nil { // Wait without Go
		t.Fatalf("second Wait = %v, want nil", err)
	}
}

// Without SetCollect (and with SetCollect(false)) Wait returns the first error itself, not a join.
func TestGroup_collectDefaultUnchanged(t *testing.T) {
	leak.Verify(t)
	for _, tc := range []struct {
		name string
		set  func(*group.Group)
	}{
		{"zero value", func(*group.Group) {}},
		{"SetCollect(false)", func(g *group.Group) { g.SetCollect(false) }},
		{"SetCollect(true) then false", func(g *group.Group) { g.SetCollect(true); g.SetCollect(false) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g group.Group
			tc.set(&g)
			g.SetLimit(1)
			g.Go(func() error { return errA })
			g.Go(func() error { return errB })
			g.Go(func() error { return errC })
			err := g.Wait()
			if err != errA { //nolint:errorlint // the default returns the very first error, unwrapped
				t.Fatalf("Wait = %v, want exactly %v", err, errA)
			}
			if errors.Is(err, errB) || errors.Is(err, errC) {
				t.Fatalf("Wait = %v matches a later error; the default keeps the first only", err)
			}
		})
	}
}

// With a limit of one, a failing function's error is recorded before its slot frees (fail runs before the
// deferred release), so the completion order is forced: the join lists the errors in that order. With no limit
// the order is whichever function returned first, which cannot be observed deterministically from outside.
func TestGroup_collectOrderIsCompletionOrder(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	var g group.Group
	g.SetCollect(true)
	g.SetLimit(1)
	want := []error{errC, errA, errB}
	submitted := make(chan struct{})
	go func() {
		defer close(submitted)
		for _, e := range want {
			g.Go(func() error { return e })
		}
	}()
	recv(t, ctx, submitted)
	got := unwrapAll(g.Wait())
	if !slices.Equal(got, want) {
		t.Fatalf("Unwrap = %v, want completion order %v", got, want)
	}
}

// SetCollect(true) after the first error: that error is still the first entry of the join.
func TestGroup_collectSetLateKeepsEarlierFirstError(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetLimit(1)
	g.Go(func() error { return errA })
	_ = g.Wait() // errA recorded while collecting was off
	g.SetCollect(true)
	g.Go(func() error { return errB })
	got := unwrapAll(g.Wait())
	if !slices.Equal(got, []error{errA, errB}) {
		t.Fatalf("Unwrap = %v, want [%v %v]", got, errA, errB)
	}
}

// The Bootcamp defect (_reference/AkhilSharma90/ULTIMATE-GO-Bootcamp-Course/small_examples/waitgroups/error.go:8-31):
// the WaitGroup and the []error are passed BY VALUE to each task, so the appends land in copies and every error is
// lost (the program does not even compile as written). The shape it wants is "five tasks, every even one fails,
// show me every failure": with SetCollect every error is seen, one per failed task.
func TestGroup_bootcampByValueWaitGroupLosesErrors(t *testing.T) {
	leak.Verify(t)
	var g group.Group
	g.SetCollect(true)
	errs := make([]error, 6)
	for id := 1; id <= 5; id++ {
		errs[id] = fmt.Errorf("task %d failed", id)
		g.Go(func() error {
			if id%2 == 0 {
				return errs[id]
			}
			return nil
		})
	}
	err := g.Wait()
	if got := unwrapAll(err); len(got) != 2 {
		t.Fatalf("Unwrap = %v, want the 2 failures (tasks 2 and 4)", got)
	}
	for _, id := range []int{2, 4} {
		if !errors.Is(err, errs[id]) {
			t.Fatalf("Wait = %v, want it to match task %d", err, id)
		}
	}
	for _, id := range []int{1, 3, 5} {
		if errors.Is(err, errs[id]) {
			t.Fatalf("Wait = %v matches task %d, which succeeded", err, id)
		}
	}
}

func ExampleWithCollect() {
	r := group.NewResults[int](context.Background(), group.WithCollect(), group.WithLimit(1))
	r.Go(func(context.Context) (int, error) { return 1, nil })
	r.Go(func(context.Context) (int, error) { return 0, errors.New("two failed") })
	// the first error cancelled the ctx: a function not yet started is not called and its slot stays zero
	r.Go(func(ctx context.Context) (int, error) { return 3, nil })
	vals, err := r.Wait()
	fmt.Println(vals)
	fmt.Println(err)
	// Output:
	// [1 0 0]
	// two failed
	// two failed
}

// runAll starts n functions on r, waits until every one is running (past the ctx check), then releases them so
// each returns what fn says; fns that have started keep their values even after the first error cancels the ctx.
func runAll[T any](t *testing.T, r *group.Results[T], n int, fn func(i int) (T, error)) {
	t.Helper()
	ctx := bounded(t)
	started := make(chan struct{}, n)
	release := make(chan struct{})
	for i := range n {
		r.Go(func(context.Context) (T, error) {
			started <- struct{}{}
			<-release
			return fn(i)
		})
	}
	for range n {
		recv(t, ctx, started)
	}
	close(release)
}

func TestResults_withCollectKeepsPartialValues(t *testing.T) {
	leak.Verify(t)
	r := group.NewResults[int](t.Context(), group.WithCollect())
	runAll(t, r, 4, func(i int) (int, error) {
		if i%2 == 1 {
			return -1, fmt.Errorf("slot %d: %w", i, errA)
		}
		return i * 10, nil
	})
	got, err := r.Wait()
	if !slices.Equal(got, []int{0, 0, 20, 0}) {
		t.Fatalf("Wait values = %v, want [0 0 20 0] (zero in the failed slots)", got)
	}
	if !errors.Is(err, errA) || len(unwrapAll(err)) != 2 {
		t.Fatalf("Wait error = %v, want a join of 2 errors wrapping %v", err, errA)
	}
}

func TestResults_withCollectZeroValueInFailedSlot(t *testing.T) {
	leak.Verify(t)
	t.Run("failed slot", func(t *testing.T) {
		r := group.NewResults[string](t.Context(), group.WithCollect())
		runAll(t, r, 3, func(i int) (string, error) {
			if i == 1 {
				return "ignored", errB // a value returned with an error is not kept
			}
			return fmt.Sprint("v", i), nil
		})
		got, err := r.Wait()
		if !slices.Equal(got, []string{"v0", "", "v2"}) || !errors.Is(err, errB) {
			t.Fatalf("Wait = %q, %v; want [v0 \"\" v2] and %v", got, err, errB)
		}
	})
	t.Run("not started after the first error", func(t *testing.T) {
		// With a limit of one the error is recorded (and the ctx cancelled) before the slot frees, so the
		// second function starts after the ctx is done: it is not called, its slot is zero, the cause is joined.
		r := group.NewResults[int](t.Context(), group.WithCollect(), group.WithLimit(1))
		r.Go(func(context.Context) (int, error) { return 0, errA })
		called := false
		r.Go(func(context.Context) (int, error) { called = true; return 2, nil })
		got, err := r.Wait()
		if called {
			t.Fatal("a function started after the first error was called")
		}
		if !slices.Equal(got, []int{0, 0}) {
			t.Fatalf("Wait values = %v, want [0 0]", got)
		}
		if u := unwrapAll(err); len(u) != 2 || u[0] != errA || !errors.Is(u[1], errA) {
			t.Fatalf("Unwrap = %v, want [errA, cause(errA)]", u)
		}
	})
}

func TestResults_withCollectPanic(t *testing.T) {
	leak.Verify(t)
	r := group.NewResults[int](t.Context(), group.WithCollect())
	runAll(t, r, 2, func(i int) (int, error) {
		if i == 1 {
			panic("boom")
		}
		return 7, nil
	})
	got, err := r.Wait()
	if !slices.Equal(got, []int{7, 0}) || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Wait = %v, %v; want [7 0] and ErrPanic", got, err)
	}
}

// Without WithCollect, Wait returns nil and the first error itself, exactly as before.
func TestResults_defaultUnchanged(t *testing.T) {
	leak.Verify(t)
	r := group.NewResults[int](t.Context(), group.WithLimit(1))
	r.Go(func(context.Context) (int, error) { return 1, nil })
	r.Go(func(context.Context) (int, error) { return 0, errA })
	r.Go(func(context.Context) (int, error) { return 0, errB })
	got, err := r.Wait()
	if got != nil || err != errA { //nolint:errorlint // the default returns the very first error, unwrapped
		t.Fatalf("Wait = %v, %v; want nil, exactly %v", got, err, errA)
	}
}

// The thinktank defect (_reference/AkhilSharma90/LLM-Projects/medium-agent-expert-gemini/thinktank.go:33-58):
// each expert's result (with its own Err) is appended under a mutex in COMPLETION order, so result i is not
// expert i. Here each function completes only after the one submitted after it (reverse order), some fail, and
// Wait still returns slot i for function i with the zero value where it failed.
func TestResults_thinktankOrderKept(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	const n = 8
	r := group.NewResults[string](ctx, group.WithCollect())
	done := make([]chan struct{}, n+1)
	for i := range done {
		done[i] = make(chan struct{})
	}
	close(done[n])
	started := make(chan struct{}, n)
	release := make(chan struct{})
	var mu sync.Mutex
	var completion []int
	for i := range n {
		r.Go(func(context.Context) (string, error) {
			started <- struct{}{}
			<-release
			select { // the group's ctx is not selected on: the first failure cancels it, and the chain decides
			case <-done[i+1]:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			mu.Lock()
			completion = append(completion, i)
			mu.Unlock()
			close(done[i])
			if i%3 == 0 {
				return "", fmt.Errorf("expert %d: %w", i, errA)
			}
			return fmt.Sprint("expert ", i), nil
		})
	}
	for range n { // every function is running, past the ctx check, before any completes
		recv(t, ctx, started)
	}
	close(release)
	got, err := r.Wait()
	// The ctx is cancelled by the first failure (expert 6, the second to complete), but every function is
	// already running and waiting on its own channel, so all of them complete, in reverse order.
	if len(completion) != n || len(got) != n {
		t.Fatalf("completed %d, got %d values, want %d", len(completion), len(got), n)
	}
	for i := range n {
		if completion[i] != n-1-i {
			t.Fatalf("completion order = %v, want reverse submission order", completion)
		}
		want := fmt.Sprint("expert ", i)
		if i%3 == 0 {
			want = ""
		}
		if got[i] != want {
			t.Fatalf("slot %d = %q, want %q (submission order kept)", i, got[i], want)
		}
	}
	if u := unwrapAll(err); len(u) != 3 || !errors.Is(err, errA) {
		t.Fatalf("Wait error = %v, want a join of the 3 failures (experts 0, 3, 6)", err)
	}
}

// errList is an uncomparable error type: comparing two of them with == panics ("comparing uncomparable type").
type errList []error

func (e errList) Error() string { return errors.Join(e...).Error() }

// Wait never compares error values: an uncomparable error returned by a function (with and without SetCollect,
// and whether it is the first error or a later one) is returned, never a panic.
func TestGroup_collectUncomparableErrorDoesNotPanic(t *testing.T) {
	leak.Verify(t)
	list := errList{errA, errB}
	for _, collect := range []bool{false, true} {
		for _, listFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("collect=%v/listFirst=%v", collect, listFirst), func(t *testing.T) {
				var g group.Group
				g.SetCollect(collect)
				g.SetLimit(1)
				if listFirst {
					g.Go(func() error { return list })
					g.Go(func() error { return errC })
				} else {
					g.Go(func() error { return errC })
					g.SetCollect(true) // the first error recorded before collecting: still first, never compared
					g.Go(func() error { return list })
				}
				err := g.Wait()
				if err == nil {
					t.Fatal("Wait = nil, want an error")
				}
				var got errList
				if !errors.As(err, &got) {
					t.Fatalf("Wait = %v, want the errList reachable by errors.As", err)
				}
				if collect || !listFirst {
					if u := unwrapAll(err); len(u) != 2 {
						t.Fatalf("Unwrap = %v, want 2 errors", u)
					}
				}
			})
		}
	}
}

// 1 000 Go calls blocked behind one slow function on a limit of one: every release closes `freed` and wakes
// every blocked caller (MATRIX §6, group.go wake), so the allocations and time per iteration measure the herd.
// The number decides the hardening item; the wakeup mechanism is unchanged in this run.
func BenchmarkGroup_Go_blockedCallers(b *testing.B) {
	b.ReportAllocs()
	const callers = 1000
	for b.Loop() {
		var g group.Group
		g.SetLimit(1)
		release := make(chan struct{})
		g.Go(func() error { <-release; return nil }) // holds the one slot
		var submitters sync.WaitGroup
		started := make(chan struct{}, callers)
		for range callers {
			submitters.Go(func() {
				started <- struct{}{}
				g.Go(func() error { return nil }) // blocks until the slot frees, then the herd wakes
			})
		}
		for range callers { // every submitter is running (most already blocked inside Go) before the release
			<-started
		}
		close(release)
		submitters.Wait()
		if err := g.Wait(); err != nil {
			b.Fatal(err)
		}
	}
}
