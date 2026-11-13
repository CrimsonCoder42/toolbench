// Package flow_test compares toolbench/flow with github.com/sourcegraph/conc/stream (v0.3.0) where the shapes
// meet: running a function over a sequence of inputs with bounded goroutines and receiving the results. Same
// inputs (1..50), same function (square).
//
// Differences, named:
//   - conc stream delivers results in submission order for any WithMaxGoroutines; flow.Stage (one worker)
//     delivers in input order, flow.FanOut (n workers) in completion order. Stage is compared value for value;
//     FanOut is compared as a sorted multiset.
//   - conc stream takes tasks with Go and callbacks; flow takes and returns channels and a wait func.
//   - a panic in a conc stream task is re-raised by Wait; flow returns panics.ErrPanic from the wait func.
//     Asserted on flow only (re-raising would end the test binary's goroutine).
//   - conc stream has no error path (a task cannot fail); flow's first error wins and cancels the rest. Not
//     compared.
package flow_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sourcegraph/conc/stream"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func square(_ context.Context, v int) (int, error) { return v * v, nil }

func inputs() []int {
	vs := make([]int, 50)
	for i := range vs {
		vs[i] = i + 1
	}
	return vs
}

func concSquares(workers int) []int {
	s := stream.New().WithMaxGoroutines(workers)
	var got []int
	for _, v := range inputs() {
		s.Go(func() stream.Callback {
			r := v * v
			return func() { got = append(got, r) }
		})
	}
	s.Wait()
	return got
}

func flowSquares(t *testing.T, ctx context.Context, workers int) []int {
	t.Helper()
	in, waitGen := flow.FromSeq(ctx, slices.Values(inputs()))
	var (
		out  <-chan int
		wait func() error
	)
	if workers == 1 {
		out, wait = flow.Stage(ctx, in, square)
	} else {
		out, wait = flow.FanOut(ctx, in, workers, square)
	}
	got := slices.Collect(flow.ToSeq(ctx, out))
	if err := errors.Join(wait(), waitGen()); err != nil {
		t.Fatalf("wait = %v", err)
	}
	return got
}

func TestStage_sameOrderAsConcStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	theirs := concSquares(1)
	ours := flowSquares(t, ctx, 1)
	if !slices.Equal(ours, theirs) {
		t.Fatalf("flow.Stage %v, conc stream %v", ours, theirs)
	}
	theirs4 := concSquares(4) // conc keeps submission order with more goroutines too
	if !slices.Equal(ours, theirs4) {
		t.Fatalf("flow.Stage %v, conc stream(4) %v", ours, theirs4)
	}
}

func TestFanOut_sameMultisetAsConcStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	theirs := concSquares(4)
	ours := slices.Sorted(slices.Values(flowSquares(t, ctx, 4)))
	if !slices.Equal(ours, theirs) { // theirs is already in ascending (submission) order
		t.Fatalf("flow.FanOut sorted %v, conc stream %v", ours, theirs)
	}
}

func TestFanOut_panicIsErrorNotRepanic(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	in, _ := flow.FromSeq(ctx, slices.Values(inputs())) // the generator cannot fail here
	out, wait := flow.FanOut(ctx, in, 4,
		func(context.Context, int) (int, error) { panic("task") })
	for range flow.ToSeq(ctx, out) {
	}
	if err := wait(); !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("wait = %v, want ErrPanic (conc stream would re-panic in Wait)", err)
	}
}
