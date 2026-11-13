package panics

import "runtime/debug"

// Try runs fn on the calling goroutine and returns fn's error unchanged, or a *PanicError (which matches
// ErrPanic) if fn panicked. A nil fn panics when called, so it too returns a *PanicError. panic(nil) is a panic.
// runtime.Goexit in fn (for example t.FailNow) is not a panic: it is not stopped, the deferred calls run and the
// goroutine ends without Try returning.
//
// ID:          TB-PANIC-003
// Pattern:     panic-to-error (recover in a deferred call)
// Authority:   Gerrand, "Defer, Panic, and Recover", The Go Blog, 2010, "Recover"
// Authority:   Effective Go, "Recover"
// Authority:   pkg.go.dev runtime, "PanicNilError" and "Goexit"
// Goroutines:  none; fn runs on the caller's goroutine
// Errors:      fn's error unchanged; a panic in fn is returned as *PanicError, errors.Is(err, ErrPanic)
// Blocking:    for as long as fn runs
// Tags:        errors, panic, recover
func Try(fn func() error) error {
	_, err := TryValue(func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

// TryValue runs fn on the calling goroutine and returns its value and error unchanged, or T's zero value and a
// *PanicError (which matches ErrPanic) if fn panicked. A nil fn returns a *PanicError. panic(nil) is a panic.
// runtime.Goexit in fn is not a panic: it is not stopped and TryValue does not return.
//
// ID:          TB-PANIC-004
// Pattern:     panic-to-error (recover in a deferred call)
// Authority:   Gerrand, "Defer, Panic, and Recover", The Go Blog, 2010, "Recover"
// Authority:   Effective Go, "Recover"
// Authority:   pkg.go.dev runtime, "PanicNilError" and "Goexit"
// Goroutines:  none; fn runs on the caller's goroutine
// Errors:      fn's error unchanged; a panic in fn is returned as *PanicError, errors.Is(err, ErrPanic)
// Blocking:    for as long as fn runs
// Tags:        errors, panic, recover, generic
func TryValue[T any](fn func() (T, error)) (T, error) {
	var (
		v        T
		err      error
		returned bool // fn returned normally
		perr     *PanicError
	)
	// The completion flags, not the recovered value, decide whether fn panicked: under GODEBUG=panicnil=1
	// recover returns nil for panic(nil), and that is still a panic. If fn calls runtime.Goexit, recover
	// returns nil, the goroutine keeps unwinding and the code after this closure never runs.
	func() {
		defer func() {
			if returned {
				return
			}
			r := recover()
			perr = &PanicError{Value: r, Stack: debug.Stack()}
		}()
		v, err = fn()
		returned = true
	}()
	if !returned {
		var zero T
		return zero, perr
	}
	return v, err
}
