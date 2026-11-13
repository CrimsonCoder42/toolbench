package semaphore

import "errors"

// ErrTooLarge is returned, wrapped with the weight and the size, by Acquire when the weight asked for exceeds
// the semaphore's size and so can never be granted. Match it with errors.Is.
//
// ID:          TB-SEMA-002
// Pattern:     sentinel error
// Authority:   pkg.go.dev golang.org/x/sync/semaphore, "func (*Weighted) Acquire" (which blocks until ctx is done instead)
// Tags:        semaphore, error
var ErrTooLarge = errors.New("semaphore: weight exceeds size")

// ErrNegativeWeight is returned, wrapped with the weight, by Acquire when the weight is negative. Match it
// with errors.Is.
//
// ID:          TB-SEMA-003
// Pattern:     sentinel error
// Authority:   pkg.go.dev golang.org/x/sync/semaphore, "type Weighted" ("a given non-negative weight"; x/sync panics instead)
// Tags:        semaphore, error
var ErrNegativeWeight = errors.New("semaphore: negative weight")
