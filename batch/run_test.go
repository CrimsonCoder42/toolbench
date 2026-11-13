package batch_test

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"testing"

	"github.com/CrimsonCoder42/toolbench/batch"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func ExampleRun() {
	err := batch.Run(context.Background(), upTo(7), 3, func(_ context.Context, b []int) error {
		fmt.Println(b)
		return nil
	})
	fmt.Println(err)
	// Output:
	// [1 2 3]
	// [4 5 6]
	// [7]
	// <nil>
}

func ExampleErrSize() {
	err := batch.Run(context.Background(), upTo(3), 0, func(context.Context, []int) error { return nil })
	fmt.Println(errors.Is(err, batch.ErrSize))
	// Output:
	// true
}

// runAll runs Run over upTo(n) and returns every batch flush received (kept as given).
func runAll(t *testing.T, ctx context.Context, n, size int) ([][]int, error) {
	t.Helper()
	var got [][]int
	err := batch.Run(ctx, upTo(n), size, func(_ context.Context, b []int) error {
		got = append(got, b)
		return nil
	})
	return got, err
}

func TestRun_sizeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		n, size int
		lens    []int
	}{
		{0, 3, nil},
		{1, 3, []int{1}},
		{2, 3, []int{2}},
		{3, 3, []int{3}},
		{4, 3, []int{3, 1}},
		{6, 3, []int{3, 3}},
		{7, 3, []int{3, 3, 1}},
		{5, 1, []int{1, 1, 1, 1, 1}},
		{5, 1 << 40, []int{5}}, // a huge size costs nothing until it fills (Chunk grows with what it sees)
	} {
		t.Run(fmt.Sprintf("n=%d,size=%d", tc.n, tc.size), func(t *testing.T) {
			got, err := runAll(t, t.Context(), tc.n, tc.size)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			var lens []int
			var flat []int
			for _, b := range got {
				lens = append(lens, len(b))
				flat = append(flat, b...)
			}
			if !slices.Equal(lens, tc.lens) {
				t.Errorf("batch sizes = %v, want %v", lens, tc.lens)
			}
			if !slices.Equal(flat, seqOf(tc.n)) {
				t.Errorf("values = %v, want 1..%d in order, each once", flat, tc.n)
			}
		})
	}
}

func TestRun_freshSlicePerBatch(t *testing.T) {
	got, err := runAll(t, t.Context(), 7, 3)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := [][]int{{1, 2, 3}, {4, 5, 6}, {7}}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("kept batches = %v, want %v (a batch was rewritten after flush returned)", got, want)
	}
	got[0][0] = 100 // the caller owns its batch; changing it touches no other batch
	if got[1][0] != 4 || got[2][0] != 7 {
		t.Errorf("batches share memory: %v", got)
	}
}

func TestRun_sizeBelowOneIsErrSize(t *testing.T) {
	for _, size := range []int{0, -1} {
		read, flushed := false, false
		in := func(func(int, error) bool) { read = true }
		err := batch.Run(t.Context(), in, size, func(context.Context, []int) error { flushed = true; return nil })
		if !errors.Is(err, batch.ErrSize) {
			t.Errorf("size %d: err = %v, want ErrSize", size, err)
		}
		if read || flushed {
			t.Errorf("size %d: in read = %v, flush called = %v; want neither", size, read, flushed)
		}
	}
}

func TestRun_nilInAndNilCtx(t *testing.T) {
	var nilCtx context.Context // documented as context.Background()
	flushed := false
	err := batch.Run(nilCtx, nil, 3, func(context.Context, []int) error { flushed = true; return nil })
	if err != nil || flushed {
		t.Errorf("nil in: err = %v, flushed = %v; want nil, false", err, flushed)
	}
	var got []int
	err = batch.Run(nilCtx, upTo(2), 3, func(_ context.Context, b []int) error { got = b; return nil })
	if err != nil || !slices.Equal(got, []int{1, 2}) {
		t.Errorf("nil ctx: err = %v, got = %v; want nil, [1 2]", err, got)
	}
}

// failingAfter yields 1 .. n, then (0, err).
func failingAfter(n int, err error) iter.Seq2[int, error] {
	return func(yield func(int, error) bool) {
		for i := 1; i <= n; i++ {
			if !yield(i, nil) {
				return
			}
		}
		yield(0, err)
	}
}

func TestRun_inErrorFlushesWhatCameBefore(t *testing.T) {
	errBad := errors.New("bad row")
	var got [][]int
	err := batch.Run(t.Context(), failingAfter(5, errBad), 3, func(_ context.Context, b []int) error {
		got = append(got, b)
		return nil
	})
	if !errors.Is(err, errBad) {
		t.Fatalf("err = %v, want errBad", err)
	}
	if want := [][]int{{1, 2, 3}, {4, 5}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("flushed %v, want %v", got, want)
	}
}

func TestRun_inErrorAtBatchBoundaryFlushesNothingMore(t *testing.T) {
	errBad := errors.New("bad row")
	calls := 0
	err := batch.Run(t.Context(), failingAfter(3, errBad), 3, func(context.Context, []int) error { calls++; return nil })
	if !errors.Is(err, errBad) || calls != 1 {
		t.Errorf("err = %v, calls = %d; want errBad, 1", err, calls)
	}
}

func TestRun_inErrorAndFlushErrorJoined(t *testing.T) {
	errBad, errDB := errors.New("bad row"), errors.New("db down")
	err := batch.Run(t.Context(), failingAfter(2, errBad), 3, func(context.Context, []int) error { return errDB })
	if !errors.Is(err, errBad) || !errors.Is(err, errDB) {
		t.Errorf("err = %v, want both errBad and errDB", err)
	}
}

// tracked yields 1 .. n and records how many values it yielded and whether its deferred cleanup ran.
type tracked struct {
	n       int
	yielded int
	cleaned bool
}

func (s *tracked) seq() iter.Seq2[int, error] {
	return func(yield func(int, error) bool) {
		defer func() { s.cleaned = true }()
		for i := 1; i <= s.n; i++ {
			s.yielded++
			if !yield(i, nil) {
				return
			}
		}
	}
}

func TestRun_flushErrorStopsIn(t *testing.T) {
	errDB := errors.New("db down")
	src := &tracked{n: 100}
	calls := 0
	err := batch.Run(t.Context(), src.seq(), 3, func(context.Context, []int) error { calls++; return errDB })
	if !errors.Is(err, errDB) {
		t.Fatalf("err = %v, want errDB", err)
	}
	if calls != 1 || src.yielded != 3 || !src.cleaned {
		t.Errorf("calls = %d, yielded = %d, cleaned = %v; want 1, 3, true", calls, src.yielded, src.cleaned)
	}
}

func TestRun_flushPanicIsErrPanic(t *testing.T) {
	src := &tracked{n: 100}
	err := batch.Run(t.Context(), src.seq(), 3, func(context.Context, []int) error { panic("boom") })
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err = %v, want ErrPanic", err)
	}
	var pe *panics.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %T, want *panics.PanicError", err)
	}
	if src.yielded != 3 || !src.cleaned {
		t.Errorf("yielded = %d, cleaned = %v; want 3, true (in stopped normally)", src.yielded, src.cleaned)
	}
}

func TestRun_nilFlushIsErrPanic(t *testing.T) {
	err := batch.Run[int](t.Context(), upTo(1), 3, nil)
	if !errors.Is(err, panics.ErrPanic) {
		t.Errorf("err = %v, want ErrPanic", err)
	}
	if err := batch.Run[int](t.Context(), upTo(0), 3, nil); err != nil {
		t.Errorf("nil flush, empty in: err = %v, want nil (flush is never called)", err)
	}
}

func TestRun_inPanicIsErrPanic(t *testing.T) {
	var got [][]int
	in := func(yield func(int, error) bool) {
		for i := 1; i <= 4; i++ {
			if !yield(i, nil) {
				return
			}
		}
		panic("in broke")
	}
	err := batch.Run(t.Context(), in, 3, func(_ context.Context, b []int) error { got = append(got, b); return nil })
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err = %v, want ErrPanic", err)
	}
	if want := [][]int{{1, 2, 3}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("flushed %v, want %v (the partial batch at the panic is dropped)", got, want)
	}
}

func TestRun_cancelBefore(t *testing.T) {
	cause := errors.New("shutdown")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	src := &tracked{n: 10}
	calls := 0
	err := batch.Run(ctx, src.seq(), 3, func(context.Context, []int) error { calls++; return nil })
	if !errors.Is(err, cause) {
		t.Errorf("err = %v, want the cause", err)
	}
	if src.yielded != 0 || calls != 0 {
		t.Errorf("yielded = %d, calls = %d; want 0, 0 (in is not read)", src.yielded, calls)
	}
}

func TestRun_cancelMidBatchDropsPartial(t *testing.T) {
	cause := errors.New("shutdown")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	cleaned := false
	in := func(yield func(int, error) bool) {
		defer func() { cleaned = true }()
		for i := 1; i <= 10; i++ {
			if i == 6 {
				cancel(cause) // values 4 and 5 are held in a partial batch
			}
			if !yield(i, nil) {
				return
			}
		}
	}
	var got [][]int
	err := batch.Run(ctx, in, 3, func(_ context.Context, b []int) error { got = append(got, b); return nil })
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want the cause", err)
	}
	if want := [][]int{{1, 2, 3}}; !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("flushed %v, want %v (partial [4 5] dropped: at-most-once)", got, want)
	}
	if !cleaned {
		t.Error("in was not stopped")
	}
}

func TestRun_cancelInsideFlushStopsAtNextBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	src := &tracked{n: 10}
	calls := 0
	err := batch.Run(ctx, src.seq(), 3, func(context.Context, []int) error {
		calls++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || !src.cleaned {
		t.Errorf("err = %v, calls = %d, cleaned = %v; want Canceled, 1, true", err, calls, src.cleaned)
	}
}

func TestRun_cancelInsideLastFlushIsNotAnError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := batch.Run(ctx, upTo(3), 3, func(context.Context, []int) error { cancel(); return nil })
	if err != nil {
		t.Errorf("err = %v, want nil: every value was flushed before ctx was seen done", err)
	}
}

func TestRun_cancelAfter(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	got, err := runAll(t, ctx, 4, 3)
	cancel()
	if err != nil || len(got) != 2 {
		t.Errorf("err = %v, batches = %d; want nil, 2", err, len(got))
	}
}

// go_38_csv reads a whole upload with csv.Reader.ReadAll before inserting: memory grows with the file. Run
// reads lazily: at no moment are more than size values read and not yet flushed.
func TestRun_go38_boundedMemoryPerBatch(t *testing.T) {
	const n, size = 10_000, 100
	produced, flushed, maxHeld := 0, 0, 0
	in := func(yield func(int, error) bool) {
		for i := range n {
			produced++
			maxHeld = max(maxHeld, produced-flushed)
			if !yield(i, nil) {
				return
			}
		}
	}
	err := batch.Run(t.Context(), in, size, func(_ context.Context, b []int) error {
		flushed += len(b)
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if flushed != n {
		t.Errorf("flushed %d values, want %d", flushed, n)
	}
	if maxHeld > size {
		t.Errorf("held %d values at once, want at most size = %d", maxHeld, size)
	}
}

// go_38_csv inserts row by row (one Exec per row): Run calls flush once per batch.
func TestRun_go38_oneFlushPerBatchNotPerRow(t *testing.T) {
	calls := 0
	err := batch.Run(t.Context(), upTo(1001), 100, func(context.Context, []int) error { calls++; return nil })
	if err != nil || calls != 11 {
		t.Errorf("err = %v, flush calls = %d; want nil, 11", err, calls)
	}
}

// go_38_csv ignores strconv errors (`loginCount, _ := strconv.Atoi(...)`): with Run a parse error is yielded
// by in and returned, matchable with errors.As.
func TestRun_go38_parseErrorNotIgnored(t *testing.T) {
	rows := []string{"1", "2", "x3", "4"}
	in := func(yield func(int, error) bool) {
		for _, r := range rows {
			v, err := strconv.Atoi(r)
			if !yield(v, err) {
				return
			}
		}
	}
	var got []int
	err := batch.Run(t.Context(), in, 10, func(_ context.Context, b []int) error { got = append(got, b...); return nil })
	var ne *strconv.NumError
	if !errors.As(err, &ne) {
		t.Fatalf("err = %v, want *strconv.NumError", err)
	}
	if !slices.Equal(got, []int{1, 2}) {
		t.Errorf("flushed %v, want [1 2] (the rows before the bad one)", got)
	}
}

func BenchmarkRun(b *testing.B) {
	b.ReportAllocs()
	flush := func(context.Context, []int) error { return nil }
	for b.Loop() {
		if err := batch.Run(context.Background(), upTo(1000), 100, flush); err != nil {
			b.Fatal(err)
		}
	}
}
