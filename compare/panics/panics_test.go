// Package panics_test compares toolbench/panics with github.com/sourcegraph/conc/panics: same inputs, same
// observable behaviour (panicked or not, the panic value, the cause reachable with errors.Is/As).
//
// Differences, named:
//   - conc.Try(func()) returns *Recovered (nil when nothing panicked); toolbench Try(func() error) returns an
//     error: fn's own error unchanged, or *PanicError.
//   - conc's ErrRecovered.Error() includes the stack; toolbench's PanicError.Error() is one line (Stack field).
//   - conc's ErrRecovered.Unwrap() gives only the cause (nil for a non-error value); toolbench's Unwrap()
//     gives ErrPanic and the cause, so errors.Is(err, ErrPanic) holds for every recovered panic.
//   - conc has Catcher (first panic across goroutines) and Repanic; toolbench ships neither (collection across
//     goroutines is the group package; re-panicking is what toolbench replaces).
package panics_test

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"testing"

	conc "github.com/sourcegraph/conc/panics"

	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestPanics_sameAsConc(t *testing.T) {
	tests := []struct {
		name     string
		fn       func()
		panicked bool
		cause    error // an error both must expose through errors.Is, or nil
	}{
		{name: "no panic", fn: func() {}},
		{name: "string", fn: func() { panic("boom") }, panicked: true},
		{name: "error", fn: func() { panic(io.EOF) }, panicked: true, cause: io.EOF},
		{name: "nil", fn: func() { panic(nil) }, panicked: true},
		{name: "runtime error", fn: func() { var m map[int]int; m[0] = 1 }, panicked: true},
		{name: "nil fn", fn: nil, panicked: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := conc.Try(tc.fn)
			err := panics.Try(func() error { tc.fn(); return nil })

			if got := rec != nil; got != tc.panicked {
				t.Fatalf("conc panicked = %v, want %v", got, tc.panicked)
			}
			if got := err != nil; got != tc.panicked {
				t.Fatalf("toolbench panicked = %v (err %v), want %v", got, err, tc.panicked)
			}
			if !tc.panicked {
				return
			}
			if !errors.Is(err, panics.ErrPanic) {
				t.Errorf("toolbench err %v does not match ErrPanic", err)
			}
			var pe *panics.PanicError
			if !errors.As(err, &pe) {
				t.Fatalf("toolbench err %T, want *PanicError", err)
			}
			// Same panic value: compare by type and by %v (a runtime.Error is a fresh value on each panic).
			if a, b := typeName(rec.Value), typeName(pe.Value); a != b {
				t.Errorf("value types differ: conc %s, toolbench %s", a, b)
			}
			if a, b := errorString(rec.Value), errorString(pe.Value); a != b {
				t.Errorf("values differ: conc %q, toolbench %q", a, b)
			}
			if len(rec.Stack) == 0 || len(pe.Stack) == 0 {
				t.Error("a stack is empty")
			}
			if tc.cause != nil {
				if !errors.Is(rec.AsError(), tc.cause) || !errors.Is(err, tc.cause) {
					t.Errorf("cause %v not reachable in both: conc %v, toolbench %v",
						tc.cause, errors.Is(rec.AsError(), tc.cause), errors.Is(err, tc.cause))
				}
			}
			// panic(nil): both see *runtime.PanicNilError (go1.21+).
			if tc.name == "nil" {
				var c, o *runtime.PanicNilError
				if !errors.As(rec.AsError(), &c) || !errors.As(err, &o) {
					t.Errorf("PanicNilError not found: conc %v, toolbench %v", c != nil, o != nil)
				}
			}
		})
	}
}

// fn's own error: conc has no notion of it (f is func()); toolbench returns it unchanged.
func TestPanics_errorPassesThrough(t *testing.T) {
	want := errors.New("fn failed")
	if err := panics.Try(func() error { return want }); err != want {
		t.Fatalf("Try = %v, want %v", err, want)
	}
	if rec := conc.Try(func() { _ = want }); rec != nil {
		t.Fatalf("conc.Try = %v, want nil", rec)
	}
}

func typeName(v any) string { return fmt.Sprintf("%T", v) }

func errorString(v any) string {
	if e, ok := v.(error); ok {
		return e.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
