package panics

import (
	"errors"
	"fmt"
)

// ErrPanic is the sentinel every recovered panic matches: errors.Is(err, ErrPanic) is true for each
// *PanicError, whatever the panic value.
//
// ID:          TB-PANIC-001
// Pattern:     panic-to-error (sentinel error)
// Authority:   Gerrand, "Defer, Panic, and Recover", The Go Blog, 2010, "Recover"
// Tags:        errors, panic, recover, sentinel
var ErrPanic = errors.New("panics: recovered panic")

// PanicError is a panic recovered from caller code: the value passed to panic and the stack of the goroutine
// that panicked, taken where the panic was recovered. It unwraps to ErrPanic and, when Value is an error, to
// Value too, so errors.Is(err, ErrPanic), errors.Is(err, io.EOF) after panic(io.EOF) and
// errors.As(err, new(runtime.Error)) all work. The zero value is usable: it prints a nil value and still
// unwraps to ErrPanic.
//
// ID:          TB-PANIC-002
// Pattern:     panic-to-error (recover in a deferred call)
// Authority:   Gerrand, "Defer, Panic, and Recover", The Go Blog, 2010, "Recover"
// Authority:   pkg.go.dev runtime, "PanicNilError" (go1.21)
// Tags:        errors, panic, recover, stack
type PanicError struct {
	// Value is the value passed to panic: a *runtime.PanicNilError for panic(nil) since go1.21, nil only
	// when the program runs with GODEBUG=panicnil=1.
	Value any
	// Stack is the panicking goroutine's stack as runtime/debug.Stack formats it.
	Stack []byte
}

// Error returns "panics: recovered panic: " and the panic value formatted with %v, on one line. The stack is
// not part of the message; read Stack.
func (e *PanicError) Error() string {
	return fmt.Sprintf("%v: %v", ErrPanic, e.Value)
}

// Unwrap returns ErrPanic, followed by Value when Value is an error, for errors.Is and errors.As.
func (e *PanicError) Unwrap() []error {
	if err, ok := e.Value.(error); ok && err != nil {
		return []error{ErrPanic, err}
	}
	return []error{ErrPanic}
}
