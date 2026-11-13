// Package singleflight_test compares toolbench/singleflight.Group with golang.org/x/sync/singleflight v0.25.0: the
// same functions passed to both give the same values, errors, shared flags and run counts, for Do, DoChan and
// Forget during a call.
//
// Known differences, by design (doc/specs/singleflight.md, deviations):
//   - A panic in fn: x/sync re-panics in the caller of Do (singleflight.go:101-102, :170) and, when a DoChan
//     caller waits, on a new goroutine nobody can recover (:166-168: "go panic(e)"), which kills the process; the
//     DoChan case cannot run here. Ours returns *panics.PanicError (panics.ErrPanic) to every caller.
//   - runtime.Goexit in fn: x/sync calls runtime.Goexit in every Do caller (:103-104); ours returns ErrGoexit.
//   - A caller's ctx: x/sync has none (a waiter cannot leave); ours returns the cause (tested in the package).
//   - DoChan's channel: x/sync never closes it; ours is closed after its one Result.
package singleflight_test

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"testing/synctest"

	xsf "golang.org/x/sync/singleflight"

	"github.com/CrimsonCoder42/toolbench/panics"
	"github.com/CrimsonCoder42/toolbench/singleflight"
)

var errA = errors.New("a failed")

type outcome struct {
	v      int
	shared bool
	err    error
}

// doer is the shape both groups are driven through: x/sync's (key string, fn) and ours, with a background ctx.
type doer struct {
	name string
	do   func(key string, fn func() (int, error)) outcome
	ch   func(key string, fn func() (int, error)) <-chan outcome
	fget func(key string)
}

func impls() []doer {
	var x xsf.Group
	var tb singleflight.Group[string, int]
	ctx := context.Background()
	return []doer{
		{
			name: "x/sync",
			do: func(key string, fn func() (int, error)) outcome {
				v, err, shared := x.Do(key, func() (any, error) { return fn() })
				n, _ := v.(int)
				return outcome{n, shared, err}
			},
			ch: func(key string, fn func() (int, error)) <-chan outcome {
				out := make(chan outcome, 1)
				r := x.DoChan(key, func() (any, error) { return fn() })
				go func() {
					res := <-r
					n, _ := res.Val.(int)
					out <- outcome{n, res.Shared, res.Err}
				}()
				return out
			},
			fget: x.Forget,
		},
		{
			name: "toolbench",
			do: func(key string, fn func() (int, error)) outcome {
				v, shared, err := tb.Do(ctx, key, func(context.Context) (int, error) { return fn() })
				return outcome{v, shared, err}
			},
			ch: func(key string, fn func() (int, error)) <-chan outcome {
				out := make(chan outcome, 1)
				r := tb.DoChan(ctx, key, func(context.Context) (int, error) { return fn() })
				go func() {
					res := <-r
					out <- outcome{res.Val, res.Shared, res.Err}
				}()
				return out
			},
			fget: tb.Forget,
		},
	}
}

// N concurrent Do on one key: fn runs once, every caller gets the value, shared is true for all.
func TestCompare_doSuppressesDuplicates(t *testing.T) {
	for _, im := range impls() {
		t.Run(im.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const n = 5
				var runs atomic.Int64
				release := make(chan struct{})
				fn := func() (int, error) { runs.Add(1); <-release; return 42, nil }
				out := make(chan outcome, n)
				for range n {
					go func() { out <- im.do("k", fn) }()
				}
				synctest.Wait()
				close(release)
				for range n {
					if o := <-out; o.v != 42 || !o.shared || o.err != nil {
						t.Errorf("Do = %+v, want 42, shared, nil", o)
					}
				}
				if runs.Load() != 1 {
					t.Fatalf("fn ran %d times, want 1", runs.Load())
				}
			})
		})
	}
}

// One caller alone: not shared; an error passes through unchanged.
func TestCompare_doAloneAndError(t *testing.T) {
	for _, im := range impls() {
		t.Run(im.name, func(t *testing.T) {
			if o := im.do("k", func() (int, error) { return 1, nil }); o.v != 1 || o.shared || o.err != nil {
				t.Fatalf("Do = %+v, want 1, false, nil", o)
			}
			if o := im.do("k", func() (int, error) { return 0, errA }); !errors.Is(o.err, errA) || o.shared {
				t.Fatalf("Do = %+v, want errA, false", o)
			}
		})
	}
}

// DoChan: two calls on one key share; Forget during a call makes the next call run fn again.
func TestCompare_doChanAndForget(t *testing.T) {
	for _, im := range impls() {
		t.Run(im.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var runs atomic.Int64
				release := make(chan struct{})
				slow := func() (int, error) { runs.Add(1); <-release; return 1, nil }
				a, b := im.ch("k", slow), im.ch("k", slow)
				synctest.Wait()
				im.fget("k")
				fresh := im.ch("k", func() (int, error) { runs.Add(1); return 2, nil })
				if o := <-fresh; o.v != 2 || o.shared || o.err != nil {
					t.Fatalf("after Forget = %+v, want 2, not shared", o)
				}
				close(release)
				for _, ch := range []<-chan outcome{a, b} {
					if o := <-ch; o.v != 1 || !o.shared || o.err != nil {
						t.Errorf("DoChan = %+v, want 1, shared", o)
					}
				}
				if runs.Load() != 2 {
					t.Fatalf("fn ran %d times, want 2", runs.Load())
				}
			})
		})
	}
}

// A panic in fn with one Do caller: x/sync re-panics in the caller (recoverable there); ours returns ErrPanic.
func TestCompare_panicInDo(t *testing.T) {
	var x xsf.Group
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _, _ = x.Do("k", func() (any, error) { panic("boom") })
	}()
	if recovered == nil {
		t.Fatal("x/sync Do did not re-panic")
	}
	var tb singleflight.Group[string, int]
	_, _, err := tb.Do(context.Background(), "k", func(context.Context) (int, error) { panic("boom") })
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("toolbench Do = %v, want ErrPanic with value boom", err)
	}
}

// runtime.Goexit in fn: x/sync ends the Do caller's goroutine; ours returns ErrGoexit to the caller.
func TestCompare_goexitInDo(t *testing.T) {
	var x xsf.Group
	returned := make(chan bool, 1)
	go func() {
		ok := false
		defer func() { returned <- ok }()
		_, _, _ = x.Do("k", func() (any, error) { runtime.Goexit(); return nil, nil })
		ok = true
	}()
	if <-returned {
		t.Fatal("x/sync Do returned after Goexit in fn")
	}
	var tb singleflight.Group[string, int]
	_, _, err := tb.Do(context.Background(), "k", func(context.Context) (int, error) { runtime.Goexit(); return 0, nil })
	if !errors.Is(err, singleflight.ErrGoexit) {
		t.Fatalf("toolbench Do = %v, want ErrGoexit", err)
	}
}
