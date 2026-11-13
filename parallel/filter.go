package parallel

import (
	"context"
	"iter"
)

// kept is one value with the verdict keep gave it.
type kept[T any] struct {
	v  T
	ok bool
}

// Filter runs keep on every value of in with at most workers goroutines and returns the values it accepted, in
// input order: the concurrent twin of calcbench algo/seq Filter (ALG-SEQ-002), which it equals for any workers
// when keep cannot fail. in is ranged once. The first error or panic from keep cancels the rest, stops in and is
// returned with nil results. workers <= 0 is runtime.GOMAXPROCS(0). A nil in gives nil, nil; a nil keep is an
// ErrPanic on the first value; a nil ctx is context.Background().
//
// ID:          TB-PAR-002
// Pattern:     bounded parallelism (parallel filter)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism"
// Authority:   pkg.go.dev iter, "Seq" and "Single-Use Iterators"
// Goroutines:  workers + 2 (the flow.FromSeq generator, the flow.FanOut workers and its closer); all have exited when Filter returns
// Cancel:      ctx done before the call: nothing is started, in is not ranged, context.Cause(ctx) is returned; during: keep sees a cancelled ctx, no new call starts, in is stopped, the cause is returned; after: no effect
// Errors:      the first keep error, or a *panics.PanicError (errors.Is panics.ErrPanic) for a panic in keep or in in, or context.Cause(ctx); results are nil on any error
// Blocking:    until every value is judged or the first error; holds every value and verdict in memory until it returns
// Tags:        concurrency, parallel, filter, predicate, workers, iterator, ordered
func Filter[T any](ctx context.Context, in iter.Seq[T], workers int, keep func(context.Context, T) (bool, error)) ([]T, error) {
	var all []kept[T]
	n := 0
	judge := func(c context.Context, v T) (kept[T], error) {
		ok, err := keep(c, v)
		return kept[T]{v, ok}, err
	}
	err := run(ctx, in, workers, judge, func(i int, k kept[T]) {
		all = place(all, i)
		all[i] = k
		if k.ok {
			n++
		}
	})
	if err != nil || n == 0 {
		return nil, err
	}
	res := make([]T, 0, n)
	for _, k := range all {
		if k.ok {
			res = append(res, k.v)
		}
	}
	return res, nil
}
