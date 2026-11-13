package config

import (
	"flag"
	"strings"
)

// FromFlags returns a Lookup over the flags of fs that were actually set on the command line: the canonical
// key "db.host" reads the flag named by FlagName ("db-host"), and its value is the flag's Value.String().
// A flag that was defined but not set is NOT found, so its flag default never overrides a lower layer (pass
// defaults through FromMap instead). fs is read at each lookup, so FromFlags may be called before fs.Parse;
// lookups must not run concurrently with fs.Parse or fs.Set (flag.FlagSet is not safe for that). A nil fs
// finds nothing.
//
// ID:          TB-CONFIG-007
// Pattern:     layered configuration (command-line layer)
// Authority:   pkg.go.dev flag, "func (*FlagSet) Visit" ("It visits only those flags that have been set") and "func (*FlagSet) Lookup", go1.27
// Tags:        config, flags, command-line
func FromFlags(fs *flag.FlagSet) Lookup {
	if fs == nil {
		return Layer()
	}
	return func(key string) (string, bool) {
		name := FlagName(key)
		if name == "" {
			return "", false
		}
		var v string
		var set bool
		fs.Visit(func(f *flag.Flag) {
			if f.Name == name {
				v, set = f.Value.String(), true
			}
		})
		return v, set
	}
}

// FlagName returns the flag name FromFlags reads for a canonical key: '.' and '_' replaced by '-'
// ("db.host" → "db-host"); case is kept.
//
// ID:          TB-CONFIG-021
// Pattern:     layered configuration (key naming across layers)
// Authority:   pkg.go.dev flag, "type Flag"
// Tags:        config, flags, naming
func FlagName(key string) string {
	return strings.NewReplacer(".", "-", "_", "-").Replace(key)
}
