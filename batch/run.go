package batch

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"

	"github.com/CrimsonCoder42/calcbench/algo/seq"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// Run reads in and calls flush with consecutive batches of size values, in input order; the last batch holds
// the remaining 1..size values and an empty in calls flush never. Every batch is a fresh slice flush may keep.
// in is consumed lazily, so at most size values are held at once whatever the length of in. Run stops at the
// first error: an error from flush is returned at once and in is stopped; an error yielded by in stops in, the
// values received before it are flushed as a final batch, and the error is returned (joined with that flush's
// error, if any). Every value in yielded before Run returns is passed to flush at most once (at-most-once):
// values held in a partial batch when ctx is done are dropped, not flushed. A nil in is empty; a nil ctx is
// context.Background().
//
// ID:          TB-BATCH-001
// Pattern:     aggregator (completeness by count)
// Authority:   Hohpe and Woolf, Enterprise Integration Patterns, 2003, "Aggregator" (Wait for All; publish the aggregate)
// Authority:   pkg.go.dev iter, "Seq2"
// Goroutines:  none; in and flush run on the caller's goroutine
// Cancel:      ctx done before the start: in is not read; between values: in is stopped, the partial batch is dropped and context.Cause(ctx) is returned; flush receives ctx
// Errors:      ErrSize for size < 1 (in is not read); flush's error; in's error (after flushing what came before it); a panic in in or flush is a *panics.PanicError (errors.Is panics.ErrPanic)
// Blocking:    for as long as in and flush run; flush is called on the caller's goroutine, so a slow flush slows the reading of in
// Math:        ALG-SEQ-004
// Tags:        batch, aggregator, iterator, iter-seq, bulk, import, size-limit
func Run[T any](ctx context.Context, in iter.Seq2[T, error], size int, flush func(context.Context, []T) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if size < 1 {
		return fmt.Errorf("%w: size = %d", ErrSize, size)
	}
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if in == nil {
		return nil
	}

	var (
		inErr     error // the error in yielded; set inside values, read after the range ends
		cancelled bool  // ctx stopped the reading of in or dropped a batch
	)
	values := func(yield func(T) bool) {
		for v, err := range in {
			if err != nil {
				inErr = err
				return
			}
			if ctx.Err() != nil {
				cancelled = true
				return
			}
			if !yield(v) {
				return
			}
		}
	}
	chunks, err := seq.Chunk(values, size) // ALG-SEQ-004; err only for size < 1, checked above
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSize, err)
	}

	var flushErr error // set when flush failed; ranging stops at once
	walkErr := panics.Try(func() error {
		for b := range chunks {
			if ctx.Err() != nil {
				cancelled = true
				return nil // the batch is dropped; the cause is returned below
			}
			// Chunk rewrites b after the loop body returns: flush gets its own copy to keep.
			flushErr = panics.Try(func() error { return flush(ctx, slices.Clone(b)) })
			if flushErr != nil {
				return nil
			}
		}
		return nil
	})
	switch {
	case walkErr != nil: // a panic in in
		return walkErr
	case flushErr != nil && inErr != nil:
		return errors.Join(inErr, flushErr)
	case flushErr != nil:
		return flushErr
	case inErr != nil:
		return inErr
	case cancelled:
		return context.Cause(ctx)
	}
	return nil
}
