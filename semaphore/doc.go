// Package semaphore is a weighted counting semaphore: Acquire takes n permits (blocking, FIFO, cancellable
// with ctx), TryAcquire takes them only if free and no one is waiting, Release gives them back. The shape is
// golang.org/x/sync/semaphore's Weighted with constructor New; unlike x/sync it never panics on input and an
// Acquire that can never be granted (n > size) returns ErrTooLarge at once instead of blocking until ctx ends.
//
// ID short form: SEMA (IDs are TB-SEMA-NNN).
//
// Exports:
//
//	TB-SEMA-001  Weighted           a weighted semaphore with FIFO waiters: New, Acquire, TryAcquire, Release
//	TB-SEMA-002  ErrTooLarge        returned by Acquire when n exceeds the semaphore's size
//	TB-SEMA-003  ErrNegativeWeight  returned by Acquire when n is negative
//	TB-SEMA-004  New                creates a Weighted with n permits; a negative n is size 0
package semaphore
