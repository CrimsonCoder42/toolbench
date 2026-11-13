package guard

import "errors"

// ErrKeys is returned by Keyed.Do and Keyed.TryDo when a new key arrives and the key bound (WithMaxKeys) is
// full, when the key is not equal to itself (a NaN) and so could never be removed from the table, or when an
// interface key holds an uncomparable value (a slice) and so cannot be a map key. It is always wrapped (the
// count of keys, or the reason); match it with errors.Is.
//
// ID:          TB-GUARD-008
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "func Is"
// Tags:        errors, sentinel, lock, per-key, bound, keys
var ErrKeys = errors.New("guard: key bound full")
