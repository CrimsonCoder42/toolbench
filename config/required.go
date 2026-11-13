package config

import (
	"errors"
	"fmt"
)

// Required reports every key of keys that is not set in l, or is set to the empty string, as one error:
// errors.Join of one error per missing key, in the order given (a key listed twice is reported once), each
// wrapping ErrMissing and naming the key. It returns nil when every key has a non-empty value. The empty
// string counts as missing because an empty required setting (DB_PASSWORD=) is the os.Getenv mistake the
// check exists to catch. A nil l reports every key.
//
// ID:          TB-CONFIG-012
// Pattern:     configuration validation (fail fast at startup, all problems at once)
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config"; pkg.go.dev errors, "func Join", go1.27
// Errors:      errors.Join of errors wrapping ErrMissing; never includes a value
// Tags:        config, validation, required, errors
func Required(l Lookup, keys ...string) error {
	var errs []error
	reported := make(map[string]bool, len(keys))
	for _, k := range keys {
		if reported[k] {
			continue
		}
		if v, ok := get(l, k); ok && v != "" {
			continue
		}
		reported[k] = true
		errs = append(errs, fmt.Errorf("%w: %q", ErrMissing, k))
	}
	return errors.Join(errs...)
}
