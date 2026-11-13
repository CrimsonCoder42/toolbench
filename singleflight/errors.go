package singleflight

import "errors"

// ErrGoexit is the error every caller of a call receives when the shared function called runtime.Goexit (for
// example t.FailNow): the goroutine running it ended without a result. x/sync calls runtime.Goexit in every
// caller instead; here the function ran on the Group's goroutine, so the callers' goroutines are not ended.
//
// ID:          TB-FLIGHT-003
// Pattern:     duplicate call suppression (singleflight)
// Authority:   golang.org/x/sync/singleflight v0.25.0 source, singleflight.go, errGoexit and doCall's double defer
// Authority:   pkg.go.dev runtime, "Goexit"
// Tags:        errors, singleflight, goexit, sentinel
var ErrGoexit = errors.New("singleflight: runtime.Goexit was called in the shared function")
