package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// Aside is the cache-aside read over any Cache: it reads k from c; on a miss it calls load, stores the
// value in c for ttl (0: c's default) and returns it. A load error is returned and nothing is stored (never
// cached, never a miss).
//
// If c fails (its Get or Set returns an error) Aside falls back to the store: on a Get error it still calls
// load, and when load succeeds it returns the value together with an error matching ErrCache and the
// cache's error, so the failure is reported but the answer is not lost. Aside has no stampede guard and no
// bloom guard (it holds no state); Memory.GetOrLoad has both. load runs on the caller's goroutine; a panic in
// it is panics.ErrPanic. ctx is passed to c and to load; if ctx is done before the call, or after a cache
// error, Aside returns context.Cause(ctx) without loading. A nil ctx is context.Background(); a nil c is a
// miss on every read (load only); a typed-nil *Memory is a failing cache (ErrCache wrapping ErrConfig, with
// the loaded value); a nil load is panics.ErrPanic.
//
// ID:          TB-CACHE-011
// Pattern:     cache-aside
// Authority:   Microsoft, "Cache-Aside pattern", Azure Architecture Center, 2025, "Solution" (steps 1-3) and "Workload design" (fall back to the store)
// Goroutines:  none
// Cancel:      ctx done before the call or after a cache error: its cause, no load; otherwise c and load honour ctx
// Errors:      load's error (nothing stored); ErrCache with a valid value when c failed; panics.ErrPanic
// Blocking:    on c and on load
// Tags:        cache, cache-aside, read-through, generic
func Aside[K comparable, V any](ctx context.Context, c Cache[K, V], k K, ttl time.Duration, load func(context.Context) (V, error)) (V, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var zero V
	if ctx.Err() != nil {
		return zero, context.Cause(ctx)
	}
	var cacheErr error
	if c != nil {
		v, ok, err := c.Get(ctx, k)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return zero, context.Cause(ctx)
			}
			cacheErr = fmt.Errorf("%w: get: %w", ErrCache, err)
		case ok:
			return v, nil
		}
	}
	v, err := panics.TryValue(func() (V, error) { return load(ctx) })
	if err != nil {
		return zero, err
	}
	if c != nil && cacheErr == nil {
		if err := c.Set(ctx, k, v, ttl); err != nil {
			cacheErr = fmt.Errorf("%w: set: %w", ErrCache, err)
		}
	}
	return v, cacheErr
}
