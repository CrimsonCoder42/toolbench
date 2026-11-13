package panics_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

func ExampleTry() {
	err := panics.Try(func() error { panic("boom") })
	fmt.Println(errors.Is(err, panics.ErrPanic))
	fmt.Println(err)

	err = panics.Try(func() error { return io.EOF })
	fmt.Println(err == io.EOF)
	// Output:
	// true
	// panics: recovered panic: boom
	// true
}

func ExampleTryValue() {
	n, err := panics.TryValue(func() (int, error) { return 42, nil })
	fmt.Println(n, err)

	n, err = panics.TryValue(func() (int, error) {
		var m map[string]int
		m["x"] = 1 // assignment to entry in nil map: a runtime panic
		return 7, nil
	})
	var re runtime.Error
	fmt.Println(n, errors.Is(err, panics.ErrPanic), errors.As(err, &re))
	// Output:
	// 42 <nil>
	// 0 true true
}

//go:noinline
func panicky() error { panic("deep") }

func TestTry_noPanic(t *testing.T) {
	if err := panics.Try(func() error { return nil }); err != nil {
		t.Fatalf("Try = %v, want nil", err)
	}
}

func TestTry_errorPassesThroughUnchanged(t *testing.T) {
	want := errors.New("fn failed")
	err := panics.Try(func() error { return want })
	if err != want {
		t.Fatalf("Try = %v, want the same error value %v", err, want)
	}
	if errors.Is(err, panics.ErrPanic) {
		t.Fatal("a returned error must not match ErrPanic")
	}
}

func TestTry_panicValues(t *testing.T) {
	cause := errors.New("cause")
	tests := []struct {
		name  string
		fn    func() error
		value any   // the expected Value, when comparable
		isErr error // an error the chain must contain besides ErrPanic
	}{
		{name: "string", fn: func() error { panic("boom") }, value: "boom"},
		{name: "int", fn: func() error { panic(7) }, value: 7},
		{name: "error", fn: func() error { panic(cause) }, value: cause, isErr: cause},
		{name: "io.EOF", fn: func() error { panic(io.EOF) }, value: io.EOF, isErr: io.EOF},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := panics.Try(tc.fn)
			if !errors.Is(err, panics.ErrPanic) {
				t.Fatalf("Try = %v, want ErrPanic", err)
			}
			var pe *panics.PanicError
			if !errors.As(err, &pe) {
				t.Fatalf("Try = %T, want *PanicError", err)
			}
			if pe.Value != tc.value {
				t.Errorf("Value = %v, want %v", pe.Value, tc.value)
			}
			if tc.isErr != nil && !errors.Is(err, tc.isErr) {
				t.Errorf("errors.Is(err, %v) = false, want true", tc.isErr)
			}
			if len(pe.Stack) == 0 {
				t.Error("Stack is empty")
			}
		})
	}
}

func TestTry_panicNil(t *testing.T) {
	err := panics.Try(func() error { panic(nil) })
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Try = %v, want ErrPanic: panic(nil) is a panic", err)
	}
	var pne *runtime.PanicNilError
	if !errors.As(err, &pne) {
		t.Fatalf("Try = %v, want the chain to hold *runtime.PanicNilError (go1.21+)", err)
	}
}

func TestTry_runtimeError(t *testing.T) {
	err := panics.Try(func() error {
		var s []int
		_ = s[3] // index out of range
		return nil
	})
	var re runtime.Error
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &re) {
		t.Fatalf("Try = %v, want ErrPanic wrapping a runtime.Error", err)
	}
}

func TestTry_nilFn(t *testing.T) {
	err := panics.Try(nil)
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Try(nil) = %v, want ErrPanic (a library function never panics on input)", err)
	}
}

func TestTry_stackNamesPanicSite(t *testing.T) {
	err := panics.Try(panicky)
	var pe *panics.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("Try = %v, want *PanicError", err)
	}
	if !bytes.Contains(pe.Stack, []byte("panics_test.panicky")) {
		t.Errorf("Stack does not name the panicking function:\n%s", pe.Stack)
	}
}

// go_27 re-panicked after cleanup; the cleanup belongs in a plain defer, and the panic arrives once, as an error.
func TestTry_cleanupRunsWithoutRepanic(t *testing.T) {
	cleaned := false
	err := panics.Try(func() error {
		defer func() { cleaned = true }()
		panic("after resource acquired")
	})
	if !cleaned {
		t.Error("the deferred cleanup inside fn did not run")
	}
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Try = %v, want ErrPanic", err)
	}
}

// backendAdvo metabolic_reports.go:577,584 runs each part as `defer func() { recover() }()`: a panicking part
// leaves its result nil and nobody learns why. With Try, the reason surfaces as ErrPanic.
func TestTry_swallowedPanicSurfacesAsErrPanic(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	parts := []func() error{
		func() error { return nil },
		func() error { var p *struct{ n int }; _ = p.n; return nil }, // nil dereference
		func() error { return nil },
	}
	errs := make([]error, len(parts))
	var wg sync.WaitGroup
	for i, part := range parts {
		wg.Go(func() { errs[i] = panics.Try(part) })
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("parts did not finish")
	}
	for i, err := range errs {
		if got, want := errors.Is(err, panics.ErrPanic), i == 1; got != want {
			t.Errorf("part %d: errors.Is(err, ErrPanic) = %v, want %v (err %v)", i, got, want, err)
		}
	}
}

// runtime.Goexit is not a panic: Try does not stop it, fn's defers run and the goroutine ends without Try
// returning. t.FailNow inside fn depends on this.
func TestTry_goexitPassesThrough(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var deferredRan, returned atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = panics.Try(func() error {
			defer deferredRan.Store(true)
			runtime.Goexit()
			return nil
		})
		returned.Store(true)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("goroutine did not exit")
	}
	if !deferredRan.Load() {
		t.Error("fn's deferred call did not run")
	}
	if returned.Load() {
		t.Error("Try returned after runtime.Goexit; it must let Goexit end the goroutine")
	}
}

func TestTryValue_noPanic(t *testing.T) {
	want := errors.New("partial")
	v, err := panics.TryValue(func() (string, error) { return "kept", want })
	if v != "kept" || err != want {
		t.Fatalf("TryValue = %q, %v; want %q, %v", v, err, "kept", want)
	}
}

func TestTryValue_panicReturnsZero(t *testing.T) {
	v, err := panics.TryValue(func() ([]int, error) { panic("boom") })
	if v != nil {
		t.Errorf("value = %v, want the zero value", v)
	}
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("TryValue = %v, want ErrPanic", err)
	}
}

func TestTryValue_nilFn(t *testing.T) {
	var fn func() (int, error)
	v, err := panics.TryValue(fn)
	if v != 0 || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("TryValue(nil) = %v, %v; want 0, ErrPanic", v, err)
	}
}

func TestTryValue_panicNil(t *testing.T) {
	_, err := panics.TryValue(func() (int, error) { panic(nil) })
	var pne *runtime.PanicNilError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pne) {
		t.Fatalf("TryValue = %v, want ErrPanic wrapping *runtime.PanicNilError", err)
	}
}

func BenchmarkTry(b *testing.B) {
	b.ReportAllocs()
	fn := func() error { return nil }
	for b.Loop() {
		_ = panics.Try(fn)
	}
}

func BenchmarkTry_panic(b *testing.B) {
	b.ReportAllocs()
	fn := func() error { panic("boom") }
	for b.Loop() {
		_ = panics.Try(fn)
	}
}
