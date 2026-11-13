package breaker

import "errors"

// ErrOpen is returned by Do and DoValue, without calling fn, when the breaker is open or when it is half-open
// and its probe calls are all in flight. Match it with errors.Is; a caller usually falls back (a cached or
// default value) or fails fast instead of retrying.
//
// ID:          TB-BRK-005
// Pattern:     sentinel error (fail fast)
// Authority:   Fowler, "CircuitBreaker", martinfowler.com, 2014 ("return with an error, without the protected call being made at all")
// Tags:        breaker, error, open, resilience
var ErrOpen = errors.New("breaker: circuit open")
