package config

import "strings"

// FromEnv returns a Lookup that reads the canonical key key from lookup under the name
//
//	prefix + strings.ToUpper(key) with every '.' and '-' replaced by '_'
//
// so with prefix "APP_" the key "db.host" reads APP_DB_HOST and "http.read-timeout" reads
// APP_HTTP_READ_TIMEOUT. The prefix is used exactly as given (include the separator). The caller passes
// os.LookupEnv in production and a map-backed func in tests; the package never reads the process
// environment itself. Present-but-empty variables are set (os.LookupEnv semantics). Because '.', '-' and '_'
// all map to '_', the keys "db.host" and "db_host" read the same variable: keep canonical keys dotted. A
// nil lookup or an empty key finds nothing.
//
// ID:          TB-CONFIG-006
// Pattern:     layered configuration (environment layer)
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config" ("stores config in environment variables"); pkg.go.dev os, "func LookupEnv", go1.27
// Tags:        config, environment, twelve-factor
func FromEnv(lookup func(string) (string, bool), prefix string) Lookup {
	if lookup == nil {
		return Layer()
	}
	return func(key string) (string, bool) {
		if key == "" {
			return "", false
		}
		return lookup(prefix + EnvName(key))
	}
}

// EnvName returns the environment variable name FromEnv reads for a canonical key, without a prefix:
// upper case, '.' and '-' replaced by '_' ("db.host" → "DB_HOST"). It is exported so a program can list the
// variables it reads.
//
// ID:          TB-CONFIG-020
// Pattern:     layered configuration (key naming across layers)
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config"
// Tags:        config, environment, naming
func EnvName(key string) string {
	return strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
}
