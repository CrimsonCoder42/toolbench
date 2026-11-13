// Package parallel holds the concurrent twins of calcbench algo/seq: Map, Filter and ForEach run a function
// over an iter.Seq with a bounded number of worker goroutines, keep input order in what they return, stop on
// the first error (cancelling the ctx the other calls see and stopping the sequence), and return a panic in
// the function or in the sequence as panics.ErrPanic. The workers are a flow.FanOut fed by flow.FromSeq.
// Sequential, lazy transforms belong to calcbench algo/seq; channel pipelines belong to package flow.
//
// ID short form: PAR (IDs are TB-PAR-NNN).
//
// Exports:
//
//	TB-PAR-001  Map      fn on every value with a bounded number of workers, results in input order, first error wins
//	TB-PAR-002  Filter   the values keep accepts, run with a bounded number of workers, in input order, first error wins
//	TB-PAR-003  ForEach  fn on every value with a bounded number of workers, first error wins
package parallel
