// Package guard keeps shared mutable state behind a stated guard (.claude/rules/go-concurrency.md, rule 8), in
// the two shapes Effective Go names: a value behind its own mutex (Guarded), and a goroutine that owns its
// state and serves requests sent on a channel (Confine, "share memory by communicating"), plus one lock per key
// (Keyed, lock striping) for callers that must not wait on each other's keys.
//
// ID short form: GUARD (IDs are TB-GUARD-NNN).
//
// Exports:
//
//	TB-GUARD-001  Guarded  a value of type T behind its own mutex: Do runs fn on it under the lock, Load copies it
//	TB-GUARD-002  Confine  serves requests on one goroutine that owns the state, until reqs closes or ctx is done
//	TB-GUARD-003  Keyed  one lock per key (lock striping): Do runs fn while no other Do on that key runs; entries leave at refcount zero
//	TB-GUARD-004  NewKeyed  an empty Keyed with options applied (the zero value is ready without it)
//	TB-GUARD-005  KeyedOption  configures NewKeyed
//	TB-GUARD-006  WithMaxKeys  bounds the distinct keys a Keyed holds or waits on at once (default 10 000)
//	TB-GUARD-007  DefaultMaxKeys  the key bound of Keyed when WithMaxKeys is not given: 10 000
//	TB-GUARD-008  ErrKeys  the sentinel for a new key when the key bound is full, or a key not equal to itself
package guard
