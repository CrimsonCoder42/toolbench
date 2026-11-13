// Package recycle is a typed object pool: sync.Pool with a type parameter, so Get returns a T and no caller
// writes a type assertion. A reset function cleans each value as it is Put, and a keep function drops values
// that should not be pooled, such as buffers that grew too large (golang/go#23199). Like sync.Pool, any pooled
// value may be removed by the garbage collector at any time, so a Pool is a cache of allocations, never a store.
// Nothing here panics on input: a nil newFn makes Get return the zero T when the pool is empty, and nil options
// are ignored. For a T that is not a pointer (a slice, a struct), every Put allocates to box the value; pool a
// pointer (*bytes.Buffer, *[]byte) to keep Get and Put allocation-free.
//
// ID short form: RECYC (IDs are TB-RECYC-NNN).
//
// Exports:
//
//	TB-RECYC-001  Pool       a typed sync.Pool: Get returns a T, Put applies keep then reset
//	TB-RECYC-002  New        creates a Pool from a constructor and options; a nil newFn means Get returns the zero T
//	TB-RECYC-003  Option     a functional option for New
//	TB-RECYC-004  WithReset  the function applied to each value on Put, before it is pooled (default none)
//	TB-RECYC-005  WithKeep   the test a value must pass on Put to be pooled; false drops it (default keep all)
package recycle
