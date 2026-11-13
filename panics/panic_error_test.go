package panics_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/CrimsonCoder42/toolbench/panics"
)

func ExampleErrPanic() {
	err := panics.Try(func() error { panic("boom") })
	if errors.Is(err, panics.ErrPanic) {
		fmt.Println("recovered:", err)
	}
	// Output:
	// recovered: panics: recovered panic: boom
}

func ExamplePanicError() {
	err := panics.Try(func() error { panic(io.EOF) })
	var pe *panics.PanicError
	if errors.As(err, &pe) {
		fmt.Println(pe.Value, len(pe.Stack) > 0)
	}
	fmt.Println(errors.Is(err, panics.ErrPanic), errors.Is(err, io.EOF))
	// Output:
	// EOF true
	// true true
}

func TestPanicError_zeroValue(t *testing.T) {
	var pe panics.PanicError
	if got, want := pe.Error(), "panics: recovered panic: <nil>"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(&pe, panics.ErrPanic) {
		t.Error("zero PanicError does not match ErrPanic")
	}
}

func TestPanicError_unwrapNonError(t *testing.T) {
	pe := &panics.PanicError{Value: "text"}
	if got := pe.Unwrap(); len(got) != 1 || got[0] != panics.ErrPanic {
		t.Errorf("Unwrap() = %v, want [ErrPanic]", got)
	}
}

func TestPanicError_unwrapError(t *testing.T) {
	pe := &panics.PanicError{Value: io.EOF}
	got := pe.Unwrap()
	if len(got) != 2 || got[0] != panics.ErrPanic || got[1] != io.EOF {
		t.Errorf("Unwrap() = %v, want [ErrPanic EOF]", got)
	}
	if errors.Is(&panics.PanicError{Value: "x"}, io.EOF) {
		t.Error("a non-error value must not match io.EOF")
	}
}

func TestPanicError_wrappedInJoin(t *testing.T) {
	err := errors.Join(errors.New("other"), panics.Try(func() error { panic("boom") }))
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) || pe.Value != "boom" {
		t.Fatalf("errors.Join chain lost the panic: %v", err)
	}
}
