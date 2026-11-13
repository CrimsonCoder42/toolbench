package retry

import "errors"

// Permanent marks err as not retryable: when fn returns it (or an error that wraps it), Do stops at once and
// returns fn's error unchanged, not ErrExhausted. The marker is transparent: Error() is err's message, and
// errors.Is and errors.As see err and everything it wraps. Permanent(nil) is nil.
//
// ID:          TB-RETRY-003
// Pattern:     non-retryable error marker (Retry pattern "Cancel" strategy)
// Authority:   Azure Architecture Center, "Retry pattern", Microsoft Learn, 2024, "Retry strategies" (Cancel)
// Errors:      returns a wrapper of err; nil for a nil err
// Tags:        retry, errors, permanent
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// permanentError is the marker Permanent returns.
type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

// isPermanent reports whether err or anything it wraps was marked by Permanent.
func isPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}
