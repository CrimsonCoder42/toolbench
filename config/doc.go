// Package config reads layered configuration without globals, reflection or environment reads of its own.
// A Lookup answers "is this key set, and to what" with os.LookupEnv's meaning; the sources (FromMap for
// defaults, FromJSON/FromJSONFile for a file through io.Reader or fs.FS, FromEnv over a lookup func the
// caller passes, FromFlags over the flags actually set) are stacked with Layer in rising precedence:
//
//	l := config.Layer(defaults, file, config.FromEnv(os.LookupEnv, "APP_"), config.FromFlags(fs))
//	// defaults < file < environment < flags
//
// Keys are canonical, lowercase and dotted ("db.host"); the environment name is APP_DB_HOST (EnvName), the
// flag is -db-host (FlagName), the file key is {"db":{"host":...}}. Typed getters (Int, Duration, Bool)
// return a default when a key is unset and ErrParse when it is set but malformed; Required lists every
// missing key at once with errors.Join. Values prints chosen keys with secrets redacted. No error message
// this package produces contains a configuration value. Nothing here starts a goroutine or blocks.
//
// ID short form: CONFIG (IDs are TB-CONFIG-NNN).
//
// Exports:
//
//	TB-CONFIG-001  Lookup        reports a key's value and whether it is set (os.LookupEnv semantics)
//	TB-CONFIG-002  Layer         stacks Lookups; a later layer overrides an earlier one
//	TB-CONFIG-003  FromMap       a Lookup over a copy of a map: the defaults layer
//	TB-CONFIG-004  FromJSON      a Lookup over one JSON object read from an io.Reader, keys flattened with '.'
//	TB-CONFIG-005  FromJSONFile  FromJSON of a file read from an fs.FS
//	TB-CONFIG-006  FromEnv       a Lookup over a caller-supplied environment lookup, PREFIX_DB_HOST naming
//	TB-CONFIG-007  FromFlags     a Lookup over the flags of a FlagSet that were set, -db-host naming
//	TB-CONFIG-008  String        a key's value, or a default when unset
//	TB-CONFIG-009  Int           a key's value as an int, a default when unset, ErrParse when malformed
//	TB-CONFIG-010  Duration      a key's value as a time.Duration, a default when unset, ErrParse when malformed
//	TB-CONFIG-011  Bool          a key's value as a bool, a default when unset, ErrParse when malformed
//	TB-CONFIG-012  Required      every key that is unset or empty, as one errors.Join of ErrMissing errors
//	TB-CONFIG-013  Values        a printable view of chosen keys with secret values never held
//	TB-CONFIG-014  Collect       reads keys from a Lookup into a redacted Values
//	TB-CONFIG-015  Option        configures Collect
//	TB-CONFIG-016  WithSecrets   marks keys as secret beyond the default words
//	TB-CONFIG-017  ErrMissing    the sentinel for a required key that is unset or empty
//	TB-CONFIG-018  ErrParse      the sentinel for a set value that does not parse as the asked type
//	TB-CONFIG-019  ErrJSON       the sentinel for JSON input that is not one flattenable object
//	TB-CONFIG-020  EnvName       the environment variable name of a canonical key, without prefix
//	TB-CONFIG-021  FlagName      the flag name of a canonical key
package config
