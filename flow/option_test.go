package flow_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/CrimsonCoder42/toolbench/flow"
)

func ExampleOption() {
	opts := []flow.Option{flow.WithBuffer(2), nil} // a nil Option is ignored
	out, wait := flow.FromSeq(context.Background(), slices.Values([]int{1}), opts...)
	fmt.Println(cap(out), <-out, wait())
	// Output:
	// 2 1 <nil>
}

func ExampleWithBuffer() {
	out, wait := flow.FromSeq(context.Background(), slices.Values([]int{1, 2}), flow.WithBuffer(-5))
	fmt.Println(cap(out)) // a negative buffer is 0: unbuffered
	for range out {
	}
	_ = wait()
	// Output:
	// 0
}

func ExampleWithRand() {
	ctx := context.Background()
	rng := rand.New(rand.NewPCG(1, 2)) // a fixed seed makes Weighted's order repeatable
	out, _ := flow.Weighted(ctx, []<-chan int{filled(1), filled(2)}, []int{1, 1}, flow.WithRand(rng))
	fmt.Println(slices.Sorted(flow.ToSeq(ctx, out)))
	// Output:
	// [1 2]
}

func ExampleErrInvalid() {
	_, err := flow.Tee[int](context.Background(), nil, 0)
	fmt.Println(errors.Is(err, flow.ErrInvalid))
	// Output:
	// true
}
