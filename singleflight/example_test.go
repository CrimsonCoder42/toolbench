package singleflight_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	"github.com/CrimsonCoder42/toolbench/singleflight"
)

// Two callers ask for the same key while the first load is in flight: the load runs once and both get its value.
func ExampleGroup() {
	var g singleflight.Group[string, int] // the zero value is ready
	ctx := context.Background()
	release := make(chan struct{})
	loads := 0
	first := g.DoChan(ctx, "user:1", func(context.Context) (int, error) {
		<-release
		loads++
		return 42, nil
	})
	second := g.DoChan(ctx, "user:1", func(context.Context) (int, error) {
		loads++ // never runs: the call for "user:1" is in flight
		return 0, nil
	})
	close(release)
	r1, r2 := <-first, <-second
	fmt.Println(r1.Val, r1.Shared, r2.Val, r2.Shared, loads)
	// Output: 42 true 42 true 1
}

func ExampleGroup_Do() {
	var g singleflight.Group[string, string]
	v, shared, err := g.Do(context.Background(), "greeting", func(context.Context) (string, error) {
		return "hello", nil
	})
	fmt.Println(v, shared, err)
	// Output: hello false <nil>
}

func ExampleGroup_DoChan() {
	var g singleflight.Group[int, int]
	ch := g.DoChan(context.Background(), 7, func(context.Context) (int, error) { return 7 * 7, nil })
	r := <-ch
	_, open := <-ch // the channel is closed after its one Result
	fmt.Println(r.Val, r.Err, r.Shared, open)
	// Output: 49 <nil> false false
}

// Forget during a call: the next caller starts a new call instead of joining the old one.
func ExampleGroup_Forget() {
	var g singleflight.Group[string, string]
	ctx := context.Background()
	release := make(chan struct{})
	old := g.DoChan(ctx, "config", func(context.Context) (string, error) {
		<-release
		return "old", nil
	})
	g.Forget("config")
	fresh := g.DoChan(ctx, "config", func(context.Context) (string, error) { return "new", nil })
	fmt.Println((<-fresh).Val)
	close(release)
	fmt.Println((<-old).Val)
	// Output:
	// new
	// old
}

func ExampleResult() {
	var g singleflight.Group[string, int]
	r := <-g.DoChan(context.Background(), "k", func(context.Context) (int, error) {
		return 0, errors.New("not found")
	})
	fmt.Printf("val=%d err=%v shared=%t\n", r.Val, r.Err, r.Shared)
	// Output: val=0 err=not found shared=false
}

func ExampleErrGoexit() {
	var g singleflight.Group[string, int]
	_, _, err := g.Do(context.Background(), "k", func(context.Context) (int, error) {
		runtime.Goexit() // for example t.FailNow inside the shared function
		return 1, nil
	})
	fmt.Println(errors.Is(err, singleflight.ErrGoexit))
	// Output: true
}
