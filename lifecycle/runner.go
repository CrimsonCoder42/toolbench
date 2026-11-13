package lifecycle

import (
	"context"
	"fmt"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// runners is the Run goroutines of one Group.Run: one per component with a non-nil Run, all started together
// once every Start returned nil. Each goroutine reports once on its own buffered channel and then on the shared
// ended channel (buffered to the goroutine count), so none ever blocks on reporting, whether or not anyone is
// still listening. Only Run's goroutine reads the fields after startRunners returns.
type runners struct {
	ctx      context.Context         // the run ctx: a child of Group.Run's ctx, cancelled when shutdown begins
	cancel   context.CancelCauseFunc // cancels ctx; the cause names who ended the group
	names    []string                // "run <name>", in Add order
	done     []chan error            // done[j] receives Run j's result once (capacity 1)
	returned []bool                  // returned[j]: done[j] has been received
	ended    chan int                // the index of each Run as it returns (capacity len(done)); nil when none
}

// startRunners derives the run ctx from ctx and starts one goroutine per non-nil Run. With no Run, ended is nil
// (a receive on it blocks forever, so the wait falls through to ctx) and nothing is started.
func startRunners(ctx context.Context, comps []Component) *runners {
	r := &runners{}
	r.ctx, r.cancel = context.WithCancelCause(ctx)
	for i, c := range comps {
		if c.Run != nil {
			r.names = append(r.names, "run "+componentName(c, i))
			r.done = append(r.done, make(chan error, 1))
		}
	}
	if len(r.done) == 0 {
		return r
	}
	r.returned = make([]bool, len(r.done))
	r.ended = make(chan int, len(r.done))
	j := 0
	for _, c := range comps {
		if c.Run == nil {
			continue
		}
		run, done, idx := c.Run, r.done[j], j
		j++
		go func() {
			err := errGoexit
			defer func() { done <- err; r.ended <- idx }()
			err = panics.Try(func() error { return run(r.ctx) })
		}()
	}
	return r
}

// first records that Run j, the first to return, ended the group: it cancels the run ctx with a cause naming
// it and returns its error wrapped with its name, or nil when the error is its own ctx's cancellation (the
// parent ctx ended the group at the same instant) or nil.
func (r *runners) first(j int) error {
	r.returned[j] = true
	err := <-r.done[j] // sent before ended was, so it is there
	if err != nil && cancelled(r.ctx, err) {
		err = nil
	}
	r.cancel(runEnded(r.names[j], err))
	if err == nil {
		return nil
	}
	return fmt.Errorf("lifecycle: %s: %w", r.names[j], err)
}

// steps returns one shutdown step per Run not yet returned, in Add order: each waits for that Run's result and
// returns its error unless it is the run ctx's cancellation. When the step ctx is done first (the grace period
// ended; shutdown has already named and abandoned the step) the step returns nil, so only the Run goroutine
// itself outlives Group.Run, until that Run returns.
func (r *runners) steps() []step {
	var steps []step
	for j := range r.done {
		if r.returned[j] {
			continue
		}
		done := r.done[j]
		steps = append(steps, step{name: r.names[j], fn: func(sctx context.Context) error {
			select {
			case err := <-done:
				if err != nil && !cancelled(r.ctx, err) {
					return err
				}
				return nil
			case <-sctx.Done():
				return nil
			}
		}})
	}
	return steps
}
