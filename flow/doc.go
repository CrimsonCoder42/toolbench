// Package flow builds concurrent pipelines out of channels: a generator from an iter.Seq and back, pipeline
// stages and bounded fan-out with first-error-wins, fan-in, the or-done, tee and bridge channels, and strict
// and weighted priority merges. Every channel flow returns is created and closed by flow exactly once; a nil
// input channel is an input with no values; cancelling ctx closes every output and every goroutine flow
// started exits; a panic in caller code becomes panics.ErrPanic. Synchronous sequences without concurrency
// belong to iter and calcbench algo/seq; concurrent Map/Filter belong to package parallel.
//
// ID short form: FLOW (IDs are TB-FLOW-NNN).
//
// Exports:
//
//	TB-FLOW-001  FromSeq           sends every value of an iter.Seq on a channel from one goroutine; wait reports a panic
//	TB-FLOW-002  ToSeq             ranges over a channel as an iter.Seq, stopping at close, ctx or break
//	TB-FLOW-003  Stage             one pipeline stage: fn on every value in input order, first error wins
//	TB-FLOW-004  FanOut            fn on every value with a fixed number of workers, completion order, first error wins
//	TB-FLOW-005  Merge             fan-in: every value of every input on one channel
//	TB-FLOW-006  OrDone            forwards a channel until it closes or ctx is done, so a range can be cancelled
//	TB-FLOW-007  Tee               copies every value of one channel to n channels, exactly once each
//	TB-FLOW-008  Bridge            flattens a channel of channels into one channel, in order
//	TB-FLOW-009  Priority          merges inputs in strict priority order, ins[0] first
//	TB-FLOW-010  Weighted          merges inputs picking a ready input with probability proportional to its weight
//	TB-FLOW-011  Option            configures FromSeq, Stage, FanOut, Tee and Weighted
//	TB-FLOW-012  WithBuffer        sets the capacity of the output channels (default 0, unbuffered)
//	TB-FLOW-013  WithRand          sets the random source Weighted picks with
//	TB-FLOW-015  ErrInvalid        the sentinel for an invalid argument to Tee or Weighted
package flow
