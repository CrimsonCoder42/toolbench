package ratelimit

import "errors"

// ErrLimit is returned (wrapped, with the values given) by NewLimiter and NewKeyed when the Limit is not
// valid: Events < 1, Per <= 0, or burst × Per in nanoseconds does not fit an int64.
//
// ID:          TB-RATE-010
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "func Is"
// Tags:        errors, sentinel, rate-limiting
var ErrLimit = errors.New("ratelimit: invalid limit")

// ErrBurst is returned (wrapped) by NewLimiter and NewKeyed when burst < 1, and by WaitN and Keyed.AllowN
// when n exceeds the burst (such a request can never be granted; x/time/rate's WaitN fails the same way).
//
// ID:          TB-RATE-011
// Pattern:     sentinel error
// Authority:   pkg.go.dev golang.org/x/time/rate, "func (*Limiter) WaitN" ("It returns an error if n exceeds the Limiter's burst size")
// Tags:        errors, sentinel, rate-limiting, burst
var ErrBurst = errors.New("ratelimit: n exceeds burst")

// ErrKeys is returned by Keyed.AllowN when a new key arrives, the key bound (WithMaxKeys) is full and no
// key is idle long enough to evict.
//
// ID:          TB-RATE-012
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "func Is"
// Tags:        errors, sentinel, rate-limiting, bound, keys
var ErrKeys = errors.New("ratelimit: key bound full")
