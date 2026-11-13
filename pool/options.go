package pool

// Option configures a Pool; pass options to New. The zero set of options is a direct hand-off (no queue)
// and collect-all errors.
//
// ID:          TB-POOL-003
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        pool, options
type Option func(*config)

type config struct {
	queue     int                   // values that may wait for a worker; 0 = direct hand-off
	failFast  bool                  // first error cancels the pool
	onDone    func(done, total int) // progress hook; nil = none
	maxErrors int                   // fn errors kept in collect mode; 0 = all
}

// WithQueue lets up to n submitted values wait for a free worker, so Submit returns before a worker takes the
// value. The default, and any n <= 0, is 0: Submit hands the value directly to a free worker and blocks until
// one takes it. The queue never grows past n: a full queue blocks Submit (backpressure, no drop).
//
// ID:          TB-POOL-004
// Pattern:     bounded queue (backpressure)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Bounded parallelism"
// Tags:        pool, options, queue, backpressure
func WithQueue(n int) Option { return func(c *config) { c.queue = max(n, 0) } }

// WithFailFast makes the first error (fn's error or a recovered panic) cancel the pool's context: workers stop
// after their current fn call, Submit returns that error, values still queued are not run (at most one per
// worker may still start, with a done ctx), and Close returns that first error, joined with a count of the
// values left unrun when there are any. Without it (the default) every error is kept, the workers keep going, and Close returns
// them joined with errors.Join.
//
// ID:          TB-POOL-005
// Pattern:     error group (first error cancels)
// Authority:   pkg.go.dev golang.org/x/sync/errgroup v0.25.0, "func WithContext"
// Tags:        pool, options, errors, cancel
func WithFailFast() Option { return func(c *config) { c.failFast = true } }

// WithOnDone sets a progress hook called after each value's fn returns, whether it succeeded, returned an error
// or panicked, with done, the number of values finished so far (this one included), and total, the number
// accepted so far by Submit and TrySubmit. Both come from the pool's own atomic counters, never from a field
// the caller reads unguarded; a Submit in flight is counted in total a moment before it returns, so done <=
// total always holds. f runs on the worker's goroutine, outside every lock, possibly from many workers at
// once, so it must be safe for concurrent use and should not block (the worker waits for it). A panic in f is
// recovered and recorded as that value's error (panics.ErrPanic, joined with fn's error when both failed).
// f is not called for a value Submit refused nor for a value left unrun by a cancelled pool. f must not call
// Close (it would wait for itself). The default, and a nil f, is no hook.
//
// ID:          TB-POOL-010
// Pattern:     observer (progress hook)
// Authority:   Gamma et al., Design Patterns, Addison-Wesley, 1994, "Observer"
// Tags:        pool, options, progress, observability, hook
func WithOnDone(f func(done, total int)) Option { return func(c *config) { c.onDone = f } }

// WithMaxErrors bounds the errors kept in collect mode: the first n fn errors (completion order) are kept, the
// rest are counted and the last of them remembered, and Close joins the kept ones with one error "pool: N
// further errors not kept" that wraps that last dropped error, so errors.Is still matches it. The default, and
// any n <= 0, keeps every error (the behaviour before this option: memory then grows one entry per failing
// value, bounded by the values submitted). It has no effect with WithFailFast, which keeps only the first error.
//
// ID:          TB-POOL-011
// Pattern:     bounded error collection
// Authority:   pkg.go.dev errors, "func Join", go1.27
// Tags:        pool, options, errors, bounded
func WithMaxErrors(n int) Option { return func(c *config) { c.maxErrors = max(n, 0) } }
