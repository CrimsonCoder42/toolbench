package parallel

import (
	"context"
	"iter"
	"slices"
)

// Map runs fn on every value of in with at most workers goroutines and returns the results in input order: the
// concurrent twin of calcbench algo/seq Map (ALG-SEQ-001), which it equals for any workers when fn cannot
// fail. in is ranged once, from one goroutine. The first error or panic from fn cancels the ctx the other calls
// see, stops in (its yield returns false, so its own cleanup runs) and is returned with nil results.
// workers <= 0 is runtime.GOMAXPROCS(0). A nil in gives nil, nil; a nil fn is an ErrPanic on the first value; a
// nil ctx is context.Background().
//
// ID:          TB-PAR-001
// Pattern:     bounded parallelism (parallel map)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism"
// Authority:   pkg.go.dev iter, "Seq" and "Single-Use Iterators"
// Goroutines:  workers + 2 (the flow.FromSeq generator, the flow.FanOut workers and its closer); all have exited when Map returns
// Cancel:      ctx done before the call: nothing is started, in is not ranged, context.Cause(ctx) is returned; during: fn sees a cancelled ctx, no new fn call starts, in is stopped, the cause is returned; after: no effect
// Errors:      the first fn error, or a *panics.PanicError (errors.Is panics.ErrPanic) for a panic in fn or in in, or context.Cause(ctx); results are nil on any error
// Blocking:    until every value is processed or the first error; holds every result in memory (len(in) slots)
// Tags:        concurrency, parallel, map, workers, iterator, ordered
func Map[In, Out any](ctx context.Context, in iter.Seq[In], workers int, fn func(context.Context, In) (Out, error)) ([]Out, error) {
	var res []Out
	err := run(ctx, in, workers, fn, func(i int, v Out) {
		res = place(res, i)
		res[i] = v
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// place returns s with length at least i+1, growing it amortized; slots beyond the old length are zero.
func place[T any](s []T, i int) []T {
	if i < len(s) {
		return s
	}
	if i >= cap(s) {
		s = slices.Grow(s, i+1-len(s))
	}
	return s[:i+1]
}
