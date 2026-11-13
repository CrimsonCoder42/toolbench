// Package singleflight suppresses duplicate calls: while a call for a key is in flight, every other caller of
// the same key waits for that one call and gets its result, instead of running the function again (a cache
// stampede becomes one load). It is golang.org/x/sync/singleflight's shape (Do, DoChan, Forget, Result) made
// generic in the key and the value and aware of context.Context:
//
//   - The shared function runs on its own goroutine with a ctx that keeps the first caller's values but not its
//     cancellation or deadline; that ctx is cancelled (cause: the last leaver's cause) only when every caller
//     waiting for the call has left.
//   - A caller whose ctx ends returns context.Cause(ctx) at once while the call continues for the others. The
//     last caller to leave cancels the call and waits for the function to return, so no goroutine outlives the
//     callers.
//   - A panic in the function is returned to every caller as panics.ErrPanic (x/sync re-panics); a
//     runtime.Goexit in it is returned as ErrGoexit (x/sync calls Goexit in every caller).
//   - A shared result is shared memory: every caller gets the same V. Do not mutate it.
//
// ID short form: FLIGHT (IDs are TB-FLIGHT-NNN).
//
// Exports:
//
//	TB-FLIGHT-001  Group      duplicate call suppression per key: Do, DoChan, Forget; the zero value is ready
//	TB-FLIGHT-002  Result     the value, error and shared flag of a call, as DoChan delivers it
//	TB-FLIGHT-003  ErrGoexit  the error every caller gets when the shared function called runtime.Goexit
package singleflight
