// Package batch groups values into batches closed by size or by time: Run reads an iter.Seq2 of values and
// errors and hands each full batch to a flush function, so an import of any length holds at most size values
// at once; Channel turns a channel of values into a channel of batches closed when size values are collected
// or maxWait after a batch's first value, whichever comes first (the Aggregator's "count or timeout"
// completeness). Every batch is a fresh slice the receiver may keep. Grouping itself is calcbench algo/seq
// Chunk (ALG-SEQ-004).
//
// ID short form: BATCH (IDs are TB-BATCH-NNN).
//
// Exports:
//
//	TB-BATCH-001  Run        calls flush with consecutive batches of size values from an iter.Seq2, stopping at the first error
//	TB-BATCH-002  Channel    sends batches of a channel's values, closed by size or maxWait after the batch's first value
//	TB-BATCH-003  Option     configures Channel
//	TB-BATCH-004  WithClock  sets the clock Channel measures maxWait with (default clock.Real)
//	TB-BATCH-005  ErrSize    the sentinel Run returns for a size below 1
package batch
