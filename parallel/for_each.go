package parallel

import (
	"context"
	"iter"
)

// ForEach runs fn on every value of in with at most workers goroutines, for its effects: the bounded,
// error-returning form of `for v := range in { fn(v) }`. Calls run in any order; in is ranged once, in order.
// The first error or panic from fn cancels the ctx the other calls see, stops in and is returned.
// workers <= 0 is runtime.GOMAXPROCS(0). A nil in is nil; a nil fn is an ErrPanic on the first value; a nil
// ctx is context.Background().
//
// ID:          TB-PAR-003
// Pattern:     bounded parallelism (parallel for-each)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism"
// Authority:   pkg.go.dev iter, "Seq"
// Goroutines:  workers + 2 (the flow.FromSeq generator, the flow.FanOut workers and its closer); all have exited when ForEach returns
// Cancel:      ctx done before the call: nothing is started, in is not ranged, context.Cause(ctx) is returned; during: fn sees a cancelled ctx, no new call starts, in is stopped, the cause is returned; after: no effect
// Errors:      the first fn error, or a *panics.PanicError (errors.Is panics.ErrPanic) for a panic in fn or in in, or context.Cause(ctx)
// Blocking:    until every value is processed or the first error; keeps no results
// Tags:        concurrency, parallel, for-each, workers, iterator
func ForEach[T any](ctx context.Context, in iter.Seq[T], workers int, fn func(context.Context, T) error) error {
	call := func(c context.Context, v T) (struct{}, error) { return struct{}{}, fn(c, v) }
	return run(ctx, in, workers, call, func(int, struct{}) {})
}
