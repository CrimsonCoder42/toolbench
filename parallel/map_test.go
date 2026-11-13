package parallel_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/CrimsonCoder42/calcbench/algo/seq"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/parallel"
)

func square(_ context.Context, v int) (int, error) { return v * v, nil }

var errBoom = errors.New("boom")

func ExampleMap() {
	words := slices.Values([]string{"go", "iter", "parallel"})
	lengths, err := parallel.Map(context.Background(), words, 3, func(_ context.Context, w string) (int, error) {
		return len(w), nil
	})
	fmt.Println(lengths, err)
	// Output:
	// [2 4 8] <nil>
}

func TestMap_cases(t *testing.T) {
	tests := []struct {
		name       string
		n, workers int
	}{
		{"zero items", 0, 4},
		{"one item", 1, 4},
		{"many items", 500, 4},
		{"workers=1", 50, 1},
		{"workers > items", 3, 10},
		{"workers=0 is GOMAXPROCS", 40, 0},
		{"workers<0 is GOMAXPROCS", 40, -3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			leak.Verify(t)
			ctx, cancel := testCtx(t)
			defer cancel()
			got, err := parallel.Map(ctx, upTo(tc.n), tc.workers, square)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			want := slices.Collect(seq.Map(upTo(tc.n), func(v int) int { return v * v }))
			if !slices.Equal(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}

// TestMap_equalsCalcbenchSeq: workers = 1 (and any other count) gives the result of the sequential twin.
func TestMap_equalsCalcbenchSeq(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	want := slices.Collect(seq.Map(upTo(100), strconv.Itoa))
	for _, w := range []int{1, 2, 7, 100, 200} {
		got, err := parallel.Map(ctx, upTo(100), w, func(_ context.Context, v int) (string, error) { return strconv.Itoa(v), nil })
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("workers=%d: got %v, %v; want %v", w, got, err, want)
		}
	}
}

// TestMap_orderKeptWhenCompletionIsReversed forces completion order 2, 1, 0 with channels; the result is still
// in input order. Also go_25's defect: its collector appends in completion order.
func TestMap_orderKeptWhenCompletionIsReversed(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	gates := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	started := make(chan int)
	finished := make(chan int)
	done := make(chan result[[]int], 1)
	go func() {
		v, err := parallel.Map(ctx, upTo(3), 3, func(c context.Context, v int) (int, error) {
			started <- v
			<-gates[v]
			finished <- v
			return v * 10, nil
		})
		done <- result[[]int]{v, err}
	}()
	for range 3 {
		await(t, ctx, started)
	}
	var completion []int
	for _, i := range []int{2, 1, 0} {
		close(gates[i])
		completion = append(completion, await(t, ctx, finished))
	}
	r := await(t, ctx, done)
	if !slices.Equal(completion, []int{2, 1, 0}) {
		t.Fatalf("completion order %v, want [2 1 0]", completion)
	}
	if r.err != nil || !slices.Equal(r.v, []int{0, 10, 20}) {
		t.Fatalf("got %v, %v; want [0 10 20] in input order", r.v, r.err)
	}
}

// TestMap_maxInFlight: the first three calls block until all three are inside fn at once, so the maximum is
// exactly workers, and never more.
func TestMap_maxInFlight(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	const workers = 3
	var (
		m       maxTracker
		calls   atomic.Int64
		started = make(chan struct{})
		release = make(chan struct{})
		done    = make(chan result[[]int], 1)
	)
	go func() {
		v, err := parallel.Map(ctx, upTo(20), workers, func(c context.Context, v int) (int, error) {
			m.enter()
			defer m.leave()
			if calls.Add(1) <= workers {
				started <- struct{}{}
				<-release
			}
			return v, nil
		})
		done <- result[[]int]{v, err}
	}()
	for range workers {
		await(t, ctx, started)
	}
	close(release)
	r := await(t, ctx, done)
	if r.err != nil || len(r.v) != 20 {
		t.Fatalf("got %d results, %v", len(r.v), r.err)
	}
	if got := m.max.Load(); got != workers {
		t.Fatalf("max in flight = %d, want %d", got, workers)
	}
}

// TestMap_go25UnboundedGoroutinePerJob: go_25 starts one goroutine per job; here 1000 jobs never have more
// than workers calls in flight.
func TestMap_go25UnboundedGoroutinePerJob(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	var m maxTracker
	got, err := parallel.Map(ctx, upTo(1000), 4, func(_ context.Context, v int) (int, error) {
		m.enter()
		defer m.leave()
		return v, nil
	})
	if err != nil || len(got) != 1000 {
		t.Fatalf("got %d results, %v", len(got), err)
	}
	if mx := m.max.Load(); mx > 4 || mx < 1 {
		t.Fatalf("max in flight = %d, want 1..4", mx)
	}
}

// TestMap_firstErrorCancelsAndStopsIn: an infinite in is stopped after the first error; the calls in flight
// see a cancelled ctx; in is advanced by a bounded number of values and its cleanup ran.
func TestMap_firstErrorCancelsAndStopsIn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	const workers = 3
	var (
		pulled    atomic.Int64
		stopped   atomic.Bool
		cancelled atomic.Int64
	)
	got, err := parallel.Map(ctx, counted(&pulled, &stopped), workers, func(c context.Context, v int) (int, error) {
		switch {
		case v == 3:
			return 0, errBoom
		case v > 3:
			<-c.Done() // held until the error cancels the rest
			cancelled.Add(1)
			return 0, c.Err()
		}
		return v, nil
	})
	if !errors.Is(err, errBoom) || got != nil {
		t.Fatalf("got %v, %v; want nil, errBoom", got, err)
	}
	if !stopped.Load() {
		t.Fatal("in was not stopped (its deferred cleanup did not run)")
	}
	// values 0..3, at most one per other worker, the one the generator holds, one received after cancel.
	if p := pulled.Load(); p > 4+workers+1 {
		t.Fatalf("in advanced %d values after an error at value 3; bound %d", p, 4+workers+1)
	}
}

func TestMap_panicInFnIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	got, err := parallel.Map(ctx, upTo(10), 3, func(_ context.Context, v int) (int, error) {
		if v == 5 {
			panic("fn broke")
		}
		return v, nil
	})
	if !errors.Is(err, panics.ErrPanic) || got != nil {
		t.Fatalf("got %v, %v; want nil, ErrPanic", got, err)
	}
}

// TestMap_go25ReadImagePanics: go_25's ReadImage panics on a bad file and takes the process down; here the
// panic is an error.
func TestMap_go25ReadImagePanics(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	read := func(_ context.Context, path string) (int, error) {
		if path == "missing.jpeg" {
			panic(errors.New("open missing.jpeg: no such file"))
		}
		return len(path), nil
	}
	_, err := parallel.Map(ctx, slices.Values([]string{"a.jpeg", "missing.jpeg", "b.jpeg"}), 2, read)
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) {
		t.Fatalf("err = %v, want a *panics.PanicError", err)
	}
}

func TestMap_panicInSeqIsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	got, err := parallel.Map(ctx, panicky(5), 2, square)
	if !errors.Is(err, panics.ErrPanic) || got != nil {
		t.Fatalf("got %v, %v; want nil, ErrPanic", got, err)
	}
}

func TestMap_cancelBefore(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("shutdown")
	cancel(cause)
	var pulled atomic.Int64
	var stopped atomic.Bool
	called := false
	got, err := parallel.Map(ctx, counted(&pulled, &stopped), 2, func(_ context.Context, v int) (int, error) {
		called = true
		return v, nil
	})
	if !errors.Is(err, cause) || got != nil {
		t.Fatalf("got %v, %v; want nil, the cause", got, err)
	}
	if pulled.Load() != 0 || called {
		t.Fatalf("pulled %d values, fn called %v; want nothing started", pulled.Load(), called)
	}
}

func TestMap_cancelMid(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancelCause(bound)
	cause := errors.New("shutdown")
	var pulled atomic.Int64
	var stopped atomic.Bool
	started := make(chan struct{}, 1)
	done := make(chan result[[]int], 1)
	go func() {
		v, err := parallel.Map(ctx, counted(&pulled, &stopped), 2, func(c context.Context, v int) (int, error) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-c.Done()
			return 0, c.Err()
		})
		done <- result[[]int]{v, err}
	}()
	await(t, bound, started)
	cancel(cause)
	r := await(t, bound, done)
	if !errors.Is(r.err, cause) || r.v != nil {
		t.Fatalf("got %v, %v; want nil, the cause", r.v, r.err)
	}
	if !stopped.Load() {
		t.Fatal("in was not stopped")
	}
}

func TestMap_cancelAfter(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	got, err := parallel.Map(ctx, upTo(5), 2, square)
	cancel()
	if err != nil || !slices.Equal(got, []int{0, 1, 4, 9, 16}) {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestMap_nilInputs(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	if got, err := parallel.Map[int, int](ctx, nil, 2, square); got != nil || err != nil {
		t.Fatalf("nil in: got %v, %v; want nil, nil", got, err)
	}
	if _, err := parallel.Map[int, int](ctx, upTo(3), 2, nil); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("nil fn: err = %v, want ErrPanic", err)
	}
	if got, err := parallel.Map(nil, upTo(3), 2, square); err != nil || !slices.Equal(got, []int{0, 1, 4}) {
		t.Fatalf("nil ctx: got %v, %v", got, err)
	}
}

// TestMap_go60IteratorNotRestartable: go_60's generator keeps its state outside the seq, so a second range
// continues where the first stopped. Map ranges in exactly once per call, so a restartable seq gives the same
// result twice and a single-use seq is still read from its first value.
func TestMap_go60IteratorNotRestartable(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	restartable := upTo(10)
	a, errA := parallel.Map(ctx, restartable, 3, square)
	b, errB := parallel.Map(ctx, restartable, 3, square)
	if errA != nil || errB != nil || !slices.Equal(a, b) {
		t.Fatalf("two calls differ: %v %v / %v %v", a, errA, b, errB)
	}
	ranges := 0
	singleUse := func(yield func(int) bool) {
		ranges++
		for i := range 4 {
			if !yield(i) {
				return
			}
		}
	}
	got, err := parallel.Map(ctx, singleUse, 3, square)
	if err != nil || ranges != 1 || !slices.Equal(got, []int{0, 1, 4, 9}) {
		t.Fatalf("got %v, %v, ranged %d times; want [0 1 4 9], nil, once", got, err, ranges)
	}
}

func BenchmarkMap(b *testing.B) {
	ctx := context.Background()
	workers := runtime.GOMAXPROCS(0)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := parallel.Map(ctx, upTo(1000), workers, square); err != nil {
			b.Fatal(err)
		}
	}
}
