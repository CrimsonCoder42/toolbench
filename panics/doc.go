// Package panics turns a panic in caller-supplied code into an error value, so a worker, a handler or a task
// that panics returns ErrPanic (with the panic value and the stack) instead of crashing the process or being
// swallowed by a bare recover. Every toolbench package that runs caller code returns a recovered panic through
// this package: errors.Is(err, panics.ErrPanic) matches it everywhere.
//
// panic(nil) is a panic (go1.21 *runtime.PanicNilError; Value is nil only under GODEBUG=panicnil=1).
// runtime.Goexit inside fn is not a panic and passes through: deferred calls run and the goroutine ends.
//
// ID short form: PANIC (IDs are TB-PANIC-NNN).
//
// Exports:
//
//	TB-PANIC-001  ErrPanic    the sentinel every recovered panic matches with errors.Is
//	TB-PANIC-002  PanicError  a recovered panic: its value and stack; unwraps to ErrPanic and an error value
//	TB-PANIC-003  Try         runs fn and returns its error, or a *PanicError if fn panicked
//	TB-PANIC-004  TryValue    runs fn and returns its value and error, or a *PanicError if fn panicked
package panics
