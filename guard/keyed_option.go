package guard

// KeyedOption configures NewKeyed. The zero value (a nil KeyedOption) is ignored. It carries the type's name so
// that an option for Guarded or Confine, should one ever exist, cannot be confused with it.
//
// ID:          TB-GUARD-005
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration, lock, per-key
type KeyedOption func(*keyedConfig)

// keyedConfig is what KeyedOptions write; NewKeyed copies it into the Keyed.
type keyedConfig struct {
	maxKeys int
}

// newKeyedConfig applies opts to the defaults, skipping nil options.
func newKeyedConfig(opts []KeyedOption) keyedConfig {
	c := keyedConfig{maxKeys: DefaultMaxKeys}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	if c.maxKeys <= 0 {
		c.maxKeys = DefaultMaxKeys
	}
	return c
}

// DefaultMaxKeys is the bound on distinct keys a Keyed holds or waits on at once when WithMaxKeys is not passed
// (or n <= 0): 10 000, the same default as ratelimit.DefaultMaxKeys. Entries leave when their last caller does,
// so this bounds concurrent use, not history.
//
// ID:          TB-GUARD-007
// Pattern:     bounded table (default bound)
// Authority:   pkg.go.dev golang.org/x/sync/singleflight, "type Group" (the keyed-table shape, there unbounded)
// Tags:        lock, per-key, bound, keys, default
const DefaultMaxKeys = 10_000

// WithMaxKeys bounds how many distinct keys a Keyed holds or waits on at once. The default, and n <= 0, is
// DefaultMaxKeys (10 000). A Do or TryDo on a new key when the bound is full is refused with ErrKeys; a key
// already in the table is never refused.
//
// ID:          TB-GUARD-006
// Pattern:     functional options; bounded table
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, lock, per-key, bound, keys
func WithMaxKeys(n int) KeyedOption {
	return func(c *keyedConfig) { c.maxKeys = n }
}
