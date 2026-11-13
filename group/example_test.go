package group_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/CrimsonCoder42/toolbench/group"
)

func ExampleGroup() {
	var g group.Group
	var sum atomic.Int64
	for _, n := range []int64{1, 2, 3} {
		g.Go(func() error { sum.Add(n * n); return nil })
	}
	if err := g.Wait(); err != nil {
		fmt.Println("error:", err)
	}
	fmt.Println(sum.Load())
	// Output: 14
}

func ExampleGroup_SetLimit() {
	var g group.Group
	g.SetLimit(2) // at most two functions run at once
	var done atomic.Int64
	for range 5 {
		g.Go(func() error { done.Add(1); return nil })
	}
	fmt.Println(g.Wait(), done.Load())
	// Output: <nil> 5
}

func ExampleGroup_TryGo() {
	var g group.Group
	g.SetLimit(1)
	release := make(chan struct{})
	fmt.Println(g.TryGo(func() error { <-release; return nil }))
	fmt.Println(g.TryGo(func() error { return nil })) // the one slot is taken
	close(release)
	fmt.Println(g.Wait())
	// Output:
	// true
	// false
	// <nil>
}

func ExampleWithContext() {
	errNotFound := errors.New("not found")
	g, ctx := group.WithContext(context.Background())
	g.Go(func() error { return errNotFound })
	g.Go(func() error {
		<-ctx.Done() // cancelled by the first error
		return context.Cause(ctx)
	})
	err := g.Wait()
	fmt.Println(err, errors.Is(context.Cause(ctx), errNotFound))
	// Output: not found true
}

func ExampleResults() {
	var r group.Results[int]
	for _, n := range []int{1, 2, 3} {
		r.Go(func(context.Context) (int, error) { return n * n, nil })
	}
	squares, err := r.Wait()
	fmt.Println(squares, err)
	// Output: [1 4 9] <nil>
}

func ExampleNewResults() {
	r := group.NewResults[string](context.Background(), group.WithLimit(2))
	for _, name := range []string{"a", "b", "c"} {
		r.Go(func(context.Context) (string, error) { return name + name, nil })
	}
	got, err := r.Wait()
	fmt.Println(got, err)
	// Output: [aa bb cc] <nil>
}

func ExampleOption() {
	opts := []group.Option{group.WithLimit(4)}
	r := group.NewResults[int](context.Background(), opts...)
	r.Go(func(context.Context) (int, error) { return 42, nil })
	fmt.Println(r.Wait())
	// Output: [42] <nil>
}

func ExampleWithLimit() {
	r := group.NewResults[int](context.Background(), group.WithLimit(1)) // one at a time
	for i := range 3 {
		r.Go(func(context.Context) (int, error) { return i, nil })
	}
	fmt.Println(r.Wait())
	// Output: [0 1 2] <nil>
}

func ExampleErrZeroLimit() {
	var g group.Group
	g.SetLimit(0)
	g.Go(func() error { return nil }) // not started; returns at once
	fmt.Println(errors.Is(g.Wait(), group.ErrZeroLimit))
	// Output: true
}
