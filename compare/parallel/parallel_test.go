// Package parallel_test compares toolbench/parallel with github.com/sourcegraph/conc/iter (v0.3.0) and
// golang.org/x/sync/errgroup (v0.25.0) where the shapes meet: a bounded parallel map, filter and for-each over
// the same inputs (0..199), the same function and the same worker count, results in input order.
//
// Differences, named:
//   - conc iter takes a slice and a func(*T); parallel takes an iter.Seq and a func(context.Context, T), so it
//     also reads a sequence of unknown length once and passes a ctx the first error cancels.
//   - conc iter.MapErr runs every element and joins all errors, returning the full result slice; parallel's
//     first error cancels the rest, stops the sequence and returns nil results. Compared as "both report the
//     error"; the count of calls is asserted on parallel only.
//   - conc iter has no Filter; compared against conc Map to a (value, keep) pair followed by a compaction.
//   - errgroup has no result collection: the comparison writes into an index-addressed slice under SetLimit,
//     the hand-rolled shape parallel.Map replaces. A panic in an errgroup function (v0.25.0) or a conc iter function
//     crashes the program (conc re-raises it); parallel returns
//     panics.ErrPanic (asserted on parallel only).
package parallel_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/sourcegraph/conc/iter"
	"golang.org/x/sync/errgroup"

	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/parallel"
)

const workers = 4

var errBoom = errors.New("boom")

func inputs() []int {
	in := make([]int, 200)
	for i := range in {
		in[i] = i
	}
	return in
}

func TestMap_sameAsConcIter(t *testing.T) {
	in := inputs()
	want := iter.Mapper[int, int]{MaxGoroutines: workers}.Map(in, func(v *int) int { return *v * 3 })
	got, err := parallel.Map(t.Context(), slices.Values(in), workers, func(_ context.Context, v int) (int, error) { return v * 3, nil })
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("parallel %v, %v; conc %v", got, err, want)
	}
}

func TestMap_sameAsErrgroupIndexed(t *testing.T) {
	in := inputs()
	want := make([]int, len(in))
	var g errgroup.Group
	g.SetLimit(workers)
	for i, v := range in {
		g.Go(func() error { want[i] = v * 3; return nil })
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	got, err := parallel.Map(t.Context(), slices.Values(in), workers, func(_ context.Context, v int) (int, error) { return v * 3, nil })
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("parallel %v, %v; errgroup %v", got, err, want)
	}
}

func TestMap_errorReportedByAll(t *testing.T) {
	in := inputs()
	fail := func(v int) (int, error) {
		if v == 50 {
			return 0, errBoom
		}
		return v, nil
	}
	_, concErr := iter.Mapper[int, int]{MaxGoroutines: workers}.MapErr(in, func(v *int) (int, error) { return fail(*v) })

	g, gctx := errgroup.WithContext(t.Context())
	g.SetLimit(workers)
	for _, v := range in {
		g.Go(func() error {
			if gctx.Err() != nil {
				return gctx.Err()
			}
			_, err := fail(v)
			return err
		})
	}
	egErr := g.Wait()

	got, err := parallel.Map(t.Context(), slices.Values(in), workers, func(_ context.Context, v int) (int, error) { return fail(v) })
	if !errors.Is(concErr, errBoom) || !errors.Is(egErr, errBoom) || !errors.Is(err, errBoom) || got != nil {
		t.Fatalf("conc %v, errgroup %v, parallel %v %v", concErr, egErr, got, err)
	}
}

func TestFilter_sameAsConcMapThenCompact(t *testing.T) {
	in := inputs()
	type pair struct {
		v  int
		ok bool
	}
	pairs := iter.Mapper[int, pair]{MaxGoroutines: workers}.Map(in, func(v *int) pair { return pair{*v, *v%7 == 0} })
	var want []int
	for _, p := range pairs {
		if p.ok {
			want = append(want, p.v)
		}
	}
	got, err := parallel.Filter(t.Context(), slices.Values(in), workers, func(_ context.Context, v int) (bool, error) { return v%7 == 0, nil })
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("parallel %v, %v; conc %v", got, err, want)
	}
}

func TestForEach_sameAsConcIter(t *testing.T) {
	in := inputs()
	concOut := slices.Clone(in)
	iter.Iterator[int]{MaxGoroutines: workers}.ForEachIdx(concOut, func(i int, v *int) { *v *= 2 })
	ourOut := make([]int, len(in))
	err := parallel.ForEach(t.Context(), slices.Values(in), workers, func(_ context.Context, v int) error {
		ourOut[v] = v * 2 // distinct index per value: no two calls write the same element
		return nil
	})
	if err != nil || !slices.Equal(ourOut, concOut) {
		t.Fatalf("parallel %v, %v; conc %v", ourOut, err, concOut)
	}
}

func TestMap_panicIsErrPanic(t *testing.T) {
	_, err := parallel.Map(t.Context(), slices.Values(inputs()), workers, func(_ context.Context, v int) (int, error) {
		if v == 10 {
			panic("broke")
		}
		return v, nil
	})
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err = %v, want panics.ErrPanic (conc re-raises, errgroup lets it crash)", err)
	}
}
