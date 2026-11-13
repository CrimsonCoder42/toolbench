package config

import "maps"

// Lookup reports the value of a configuration key and whether the key is set, with the meaning of
// os.LookupEnv: a key set to the empty string is ("", true), an unset key is ("", false). Keys are canonical:
// lowercase, dot-separated segments ("db.host"); each source maps the canonical key to its own naming
// (FromEnv: DB_HOST, FromFlags: -db-host, FromJSON: {"db":{"host":...}}). The zero value (a nil Lookup) is
// accepted by every function here and finds nothing. The Lookups this package returns are immutable or read
// their source only, and are safe for concurrent use.
//
// ID:          TB-CONFIG-001
// Pattern:     layered configuration source (key lookup)
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config"; pkg.go.dev os, "func LookupEnv", go1.27
// Tags:        config, lookup, environment, twelve-factor
type Lookup func(key string) (string, bool)

// Layer returns a Lookup that asks ls from LAST to first and returns the first answer that is set: a later
// layer overrides an earlier one, so the arguments read in rising precedence,
//
//	config.Layer(defaults, file, env, flags) // defaults < file < environment < flags
//
// A key set to the empty string in a later layer overrides a non-empty value below it (it is set). Nil
// layers are skipped; Layer() finds nothing. The slice is copied, so changing ls afterwards has no effect.
//
// ID:          TB-CONFIG-002
// Pattern:     layered configuration (precedence chain)
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config" (environment over code); pkg.go.dev flag, "func (*FlagSet) Visit"
// Tags:        config, layering, precedence, twelve-factor
func Layer(ls ...Lookup) Lookup {
	layers := make([]Lookup, 0, len(ls))
	for _, l := range ls {
		if l != nil {
			layers = append(layers, l)
		}
	}
	return func(key string) (string, bool) {
		for i := len(layers) - 1; i >= 0; i-- {
			if v, ok := layers[i](key); ok {
				return v, true
			}
		}
		return "", false
	}
}

// FromMap returns a Lookup over a copy of m, keyed by canonical key ("db.port": "5432"); it is the defaults
// layer. Changing m afterwards has no effect. A nil m finds nothing.
//
// ID:          TB-CONFIG-003
// Pattern:     layered configuration (defaults layer)
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config"
// Tags:        config, defaults, map
func FromMap(m map[string]string) Lookup {
	c := maps.Clone(m)
	return func(key string) (string, bool) {
		v, ok := c[key]
		return v, ok
	}
}
