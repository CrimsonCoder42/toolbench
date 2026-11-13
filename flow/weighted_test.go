package flow_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExampleWeighted() {
	ctx := context.Background()
	gold, bronze := filled("g", "g", "g"), filled("b", "b")
	out, err := flow.Weighted(ctx, []<-chan string{gold, bronze}, []int{3, 1}, flow.WithRand(rand.New(rand.NewPCG(1, 2))))
	if err != nil {
		fmt.Println(err)
		return
	}
	counts := map[string]int{}
	for v := range out {
		counts[v]++
	}
	fmt.Println("g:", counts["g"], "b:", counts["b"]) // every value is delivered; the order is weighted random
	// Output:
	// g: 3 b: 2
}

func repeat(tag, n int) []int {
	vs := make([]int, n)
	for i := range vs {
		vs[i] = tag
	}
	return vs
}

func TestWeighted_fixedSeedDistribution(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	ins := []<-chan int{filledOpen(repeat(0, 4000)...), filledOpen(repeat(1, 4000)...)}
	out, err := flow.Weighted(ctx, ins, []int{3, 1}, flow.WithRand(rand.New(rand.NewPCG(1, 2))))
	if err != nil {
		t.Fatal(err)
	}
	heavy := 0
	for range 2000 {
		if <-out == 0 {
			heavy++
		}
	}
	cancel()
	collect(t, bound, out)
	// Expected 1500 of 2000 (weight 3/4); the standard deviation is about 19.
	if heavy < 1350 || heavy > 1650 {
		t.Fatalf("heavy input got %d of 2000 turns, want about 1500", heavy)
	}
}

func TestWeighted_sameSeedSameOrder(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	run := func() []int {
		ins := []<-chan int{filled(repeat(0, 200)...), filled(repeat(1, 200)...), filled(repeat(2, 200)...)}
		out, err := flow.Weighted(ctx, ins, []int{5, 2, 1}, flow.WithRand(rand.New(rand.NewPCG(7, 7))))
		if err != nil {
			t.Fatal(err)
		}
		return collect(t, ctx, out)
	}
	a, b := run(), run()
	if !slices.Equal(a, b) {
		t.Fatal("the same seed and inputs gave different orders")
	}
	counts := make([]int, 3)
	for _, v := range a {
		counts[v]++
	}
	if !slices.Equal(counts, []int{200, 200, 200}) {
		t.Fatalf("counts %v, want every value delivered", counts)
	}
}

func TestWeighted_workConservingAndNil(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := testCtx(t)
	defer cancel()
	// The heavy input is open but empty: the light one is never held back waiting for it.
	heavyEmpty := make(chan int)
	defer close(heavyEmpty)
	out, err := flow.Weighted(ctx, []<-chan int{heavyEmpty, nil, filledOpen(1, 2, 3)}, []int{100, 1, 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []int{1, 2, 3} {
		if v := <-out; v != want {
			t.Fatalf("got %d, want %d", v, want)
		}
	}
	cancel()
	collect(t, t.Context(), out)
}

func TestWeighted_invalid(t *testing.T) {
	one := []<-chan int{filled(1)}
	two := []<-chan int{filled(1), filled(2)}
	tests := []struct {
		name    string
		ins     []<-chan int
		weights []int
	}{
		{"no inputs", nil, nil},
		{"count mismatch", two, []int{1}},
		{"zero weight", two, []int{1, 0}},
		{"negative weight", one, []int{-1}},
		{"overflow", two, []int{math.MaxInt, 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := flow.Weighted(t.Context(), tc.ins, tc.weights)
			if !errors.Is(err, flow.ErrInvalid) || out != nil {
				t.Fatalf("got %v, %v, want nil, ErrInvalid", out, err)
			}
		})
	}
}

func TestWeighted_cancelBefore(t *testing.T) {
	leak.Verify(t)
	bound, cancelBound := testCtx(t)
	defer cancelBound()
	ctx, cancel := context.WithCancel(bound)
	cancel()
	out, err := flow.Weighted(ctx, []<-chan int{filled(1, 2)}, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if got := collect(t, bound, out); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
