package flow

import "errors"

// ErrInvalid is returned, wrapped with the argument it names, when Tee or Weighted is called with arguments it
// cannot run with (n < 1, mismatched or empty weights, a weight below 1, a total that overflows). Nothing is
// started when it is returned. Match it with errors.Is.
//
// ID:          TB-FLOW-015
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "Is"
// Tags:        errors, sentinel
var ErrInvalid = errors.New("flow: invalid argument")
