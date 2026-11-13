package recycle

// Option changes how New builds a Pool. A nil Option is ignored.
//
// ID:          TB-RECYC-003
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        pool, options
type Option[T any] func(*config[T])

type config[T any] struct {
	reset func(T) T
	keep  func(T) bool
}

// WithReset sets the function Put applies to each value it keeps, before pooling it; its result is what is
// pooled, so a slice can be cut to length 0 (b[:0]) and a pointer can be cleared in place and returned. By
// default (or with a nil reset) values are pooled unchanged. reset runs after keep, and only for kept values.
//
// ID:          TB-RECYC-004
// Pattern:     object pool (reset on release)
// Authority:   pkg.go.dev sync, "type Pool" ("cache allocated but unused items for later reuse")
// Tags:        pool, reset, options
func WithReset[T any](reset func(T) T) Option[T] {
	return func(c *config[T]) { c.reset = reset }
}

// WithKeep sets the test Put applies to each value: a value for which keep returns false is dropped, not
// pooled, so one oversized buffer cannot stay pinned in the pool and be handed to callers that need a small
// one (golang/go#23199; cap the capacity, for example keep = func(b *bytes.Buffer) bool { return b.Cap() <=
// 64<<10 }). By default (or with a nil keep) every value is pooled.
//
// ID:          TB-RECYC-005
// Pattern:     object pool (size-gated release)
// Authority:   golang/go#23199, "sync: Pool example suggests incorrect usage", 2017 (size-gating)
// Tags:        pool, buffer, memory, options
func WithKeep[T any](keep func(T) bool) Option[T] {
	return func(c *config[T]) { c.keep = keep }
}
