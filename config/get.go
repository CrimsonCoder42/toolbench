package config

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// String returns the value of key in l, or def when the key is not set. A key set to the empty string
// returns "" (it is set). A nil l returns def. String never fails.
//
// ID:          TB-CONFIG-008
// Pattern:     typed configuration getter
// Authority:   pkg.go.dev os, "func LookupEnv" (set vs unset), go1.27
// Tags:        config, getter, string
func String(l Lookup, key, def string) string {
	if v, ok := get(l, key); ok {
		return v
	}
	return def
}

// Int returns the value of key in l parsed as a base-10 int (strconv.ParseInt(v, 10, 0)), or def with a nil
// error when the key is not set. A set value that does not parse, including the empty string, returns 0
// and an error wrapping ErrParse and strconv.ErrSyntax or strconv.ErrRange; the message names the key,
// never the value. A nil l returns def.
//
// ID:          TB-CONFIG-009
// Pattern:     typed configuration getter
// Authority:   pkg.go.dev strconv, "func ParseInt", go1.27
// Errors:      ErrParse (with strconv.ErrSyntax or strconv.ErrRange)
// Tags:        config, getter, int, parse
func Int(l Lookup, key string, def int) (int, error) {
	v, ok := get(l, key)
	if !ok {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 0)
	if err != nil {
		return 0, parseError(key, "int", err)
	}
	return int(n), nil
}

// Duration returns the value of key in l parsed by time.ParseDuration ("1m30s", "250ms"; a bare number
// other than "0" is an error), or def with a nil error when the key is not set. A set value that does not
// parse, including the empty string, returns 0 and an error wrapping ErrParse; the message names the key,
// never the value. A nil l returns def.
//
// ID:          TB-CONFIG-010
// Pattern:     typed configuration getter
// Authority:   pkg.go.dev time, "func ParseDuration", go1.27
// Errors:      ErrParse
// Tags:        config, getter, duration, parse
func Duration(l Lookup, key string, def time.Duration) (time.Duration, error) {
	v, ok := get(l, key)
	if !ok {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		// time's error quotes the input: replace it so the value never reaches the message.
		return 0, parseError(key, "duration", strconv.ErrSyntax)
	}
	return d, nil
}

// Bool returns the value of key in l parsed by strconv.ParseBool (1, t, T, TRUE, true, True, 0, f, F,
// FALSE, false, False), or def with a nil error when the key is not set. A set value that does not parse,
// including the empty string, returns false and an error wrapping ErrParse and strconv.ErrSyntax; the
// message names the key, never the value. A nil l returns def.
//
// ID:          TB-CONFIG-011
// Pattern:     typed configuration getter
// Authority:   pkg.go.dev strconv, "func ParseBool", go1.27
// Errors:      ErrParse (with strconv.ErrSyntax)
// Tags:        config, getter, bool, parse
func Bool(l Lookup, key string, def bool) (bool, error) {
	v, ok := get(l, key)
	if !ok {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, parseError(key, "bool", err)
	}
	return b, nil
}

func get(l Lookup, key string) (string, bool) {
	if l == nil {
		return "", false
	}
	return l(key)
}

// parseError names the key and the failure kind; a *strconv.NumError (whose message quotes the input) is
// unwrapped to its bare cause so the value is never in the text.
func parseError(key, kind string, err error) error {
	if ne, ok := errors.AsType[*strconv.NumError](err); ok {
		err = ne.Err
	}
	return fmt.Errorf("%w: key %q is not a valid %s: %w", ErrParse, key, kind, err)
}
