package guard

import (
	"context"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// Confine makes the calling goroutine the only owner of state: it receives requests from reqs and runs each
// one, in arrival order, with a pointer to its copy of state, until reqs is closed, ctx is done or a request
// panics, and then returns the final state. A request that needs an answer closes over its own reply channel.
// A nil reqs is handled as a closed one; a nil request is skipped; a nil ctx is context.Background(). Run it
// as go func() { s, err = guard.Confine(ctx, s0, reqs) }() for a separate owner goroutine; that goroutine
// ends when Confine returns.
//
// The *S a request receives is valid only while it runs (keeping it is a data race). The returned S is a
// shallow copy of the final state.
//
// ID:          TB-GUARD-002
// Pattern:     confinement (one goroutine owns the state; others send it requests)
// Authority:   Effective Go, go.dev, "Concurrency", "Share by communicating"
// Authority:   Cox-Buday, Concurrency in Go, O'Reilly, 2017, ch.4, "Confinement"
// Goroutines:  none; the loop runs on the caller's goroutine and has returned when Confine returns
// Cancel:      ctx done: returns at once when waiting, or after the request it has received finishes
// Cancel:      a request already received runs to completion; no request is received after that
// Errors:      nil when reqs is closed (every request sent ran) or nil; context.Cause(ctx) when ctx is done
// Errors:      a panic in a request stops the loop: the state as the panic left it and a *panics.PanicError
// Blocking:    until reqs is closed, ctx is done or a request panics; senders block until the owner receives
// Tags:        concurrency, confinement, channel, owner, shared-state, generic
func Confine[S any](ctx context.Context, state S, reqs <-chan func(*S)) (S, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if reqs == nil {
		return state, nil
	}
	for {
		if ctx.Err() != nil {
			return state, context.Cause(ctx)
		}
		select {
		case <-ctx.Done():
			return state, context.Cause(ctx)
		case fn, ok := <-reqs:
			if !ok {
				return state, nil
			}
			// A received request always runs: its sender's send has completed and may be waiting on a reply.
			// The ctx check at the top of the loop stops the next receive.
			if fn == nil {
				continue
			}
			if err := panics.Try(func() error { fn(&state); return nil }); err != nil {
				return state, err
			}
		}
	}
}
