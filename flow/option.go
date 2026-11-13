package flow

import "math/rand/v2"

// Option configures FromSeq, Stage, FanOut, Tee and Weighted. Each function ignores the options that do not
// apply to it. The zero value (a nil Option) is ignored.
//
// ID:          TB-FLOW-011
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration
type Option func(*config)

type config struct {
	buffer int
	rng    *rand.Rand
}

func newConfig(opts []Option) config {
	var c config
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}

// random returns the configured random source, or a new PCG source seeded from math/rand/v2 for this call.
func (c config) random() *rand.Rand {
	if c.rng != nil {
		return c.rng
	}
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}

// WithBuffer sets the capacity of each output channel. The default is 0: unbuffered, so a slow consumer slows
// the producer (backpressure). A negative n is 0. A buffer only adds slack; values are never dropped.
//
// ID:          TB-FLOW-012
// Pattern:     buffered channel (bounded buffer)
// Authority:   The Go Programming Language Specification, "Channel types"
// Tags:        options, backpressure, buffer
func WithBuffer(n int) Option {
	return func(c *config) { c.buffer = max(n, 0) }
}

// WithRand sets the random source Weighted picks with. It is used from one goroutine only, so it must not be
// shared with other goroutines while the merge runs. nil means the default: a new PCG source per call, seeded
// from math/rand/v2. Tests pass a fixed seed, for example rand.New(rand.NewPCG(1, 2)).
//
// ID:          TB-FLOW-013
// Pattern:     dependency injection (injected random source)
// Authority:   pkg.go.dev math/rand/v2, "Rand" and "NewPCG"
// Tags:        options, random, testing
func WithRand(r *rand.Rand) Option {
	return func(c *config) { c.rng = r }
}
