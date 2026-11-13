package group

// SetCollect chooses what Wait returns when more than one function fails. With collect true, every function
// error, every recovered panic (panics.ErrPanic) and ErrZeroLimit are kept, one entry per failed Go or TryGo in
// the order they completed, and Wait returns them joined with errors.Join (nil when none failed; a single
// failure is still wrapped by Join, so match with errors.Is or errors.As, never ==). With false, the default,
// Wait returns the first error only, as errgroup. Collecting changes what Wait returns, not when the group stops:
// the first error still cancels a WithContext ctx with itself as the cause; a caller who wants every function to
// run to completion uses a Group without a ctx (the zero Group does not cancel on error). Call it before the
// first Go, as errgroup says of SetLimit; a later call applies to errors recorded afterwards, and a first error
// recorded before it is still the first entry of the join.
//
// ID:          TB-GROUP-008
// Pattern:     error group (collect-all errors)
// Authority:   pkg.go.dev errors, "func Join", go1.27
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func (*Group) SetLimit"
// Goroutines:  none
// Errors:      with collect true Wait returns errors.Join of every error in completion order; false: the first only
// Blocking:    none
// Tags:        concurrency, errgroup, errors, join, collect
func (g *Group) SetCollect(collect bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.collect = collect
}

// WithCollect makes a Results keep partial results: Wait returns one value per Go in submission order, with the
// zero value in every slot whose function returned an error, panicked or was not called because the context was
// already done, and the error of every failure joined with errors.Join (Group.SetCollect underneath). The first
// error still cancels the context, so a function that has not started when it fires is not called and its slot
// holds the zero value with the context's cause in the join; a function already running that then succeeds
// keeps its value. Without the option Wait returns nil and the first error.
//
// ID:          TB-GROUP-009
// Pattern:     result group (partial results, collect-all errors)
// Authority:   pkg.go.dev errors, "func Join", go1.27
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func (*Group) SetLimit"
// Errors:      Wait returns the values of the Go calls that succeeded and errors.Join of every failure
// Tags:        concurrency, errgroup, results, errors, join, options
func WithCollect() Option {
	return func(c *config) { c.collect = true }
}
