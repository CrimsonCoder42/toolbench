// Package group runs a set of functions on their own goroutines and waits for all of them: the error group
// (golang.org/x/sync/errgroup's shape: Go, TryGo, SetLimit, Wait, WithContext) and the result group (typed
// results in submission order). The first error wins and cancels the derived context; Wait returns only after
// every started goroutine has returned. A panic in a function is recovered and returned through Wait as
// panics.ErrPanic with its stack, never re-raised and never a crash. Nothing here panics or deadlocks on input:
// SetLimit while goroutines are active applies to later calls, and a limit of zero is reported as ErrZeroLimit.
// SetCollect (Group) and WithCollect (Results) are the opt-in to keep every error joined with errors.Join; Results
// then returns partial values with the zero value in a failed slot.
//
// ID short form: GROUP (IDs are TB-GROUP-NNN).
//
// Exports:
//
//	TB-GROUP-001  Group         an error group: first error wins and cancels, optional limit, panics become ErrPanic
//	TB-GROUP-002  WithContext   a new Group and a ctx cancelled on its first error or when Wait returns
//	TB-GROUP-003  Results       a result group: one value per Go, returned by Wait in submission order
//	TB-GROUP-004  NewResults    a Results bound to a ctx, with options
//	TB-GROUP-005  Option        a functional option for NewResults
//	TB-GROUP-006  WithLimit     the most goroutines a Results runs at once (default no limit)
//	TB-GROUP-007  ErrZeroLimit  the error recorded when Go is called while the limit is zero
//	TB-GROUP-008  Group.SetCollect  Wait returns every error joined with errors.Join, completion order (default: first only)
//	TB-GROUP-009  WithCollect   Results option: Wait returns partial values (zero in a failed slot) and every error joined
package group
