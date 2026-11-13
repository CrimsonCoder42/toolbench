package batch

import "errors"

// ErrSize is returned by Run, wrapped with the size given, when size is below 1. Match it with errors.Is.
//
// ID:          TB-BATCH-005
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "Is"
// Tags:        errors, batch, validation
var ErrSize = errors.New("batch: size must be at least 1")
