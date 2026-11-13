//go:debug panicnil=1

// Package panicniltest runs panics.Try with GODEBUG=panicnil=1 set for this test binary only (the //go:debug
// line above), where recover returns nil for panic(nil). It holds tests only and exports nothing.
package panicniltest

import (
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// recoverNil reports whether recover returns nil for panic(nil), i.e. whether panicnil=1 is in effect.
func recoverNil() (isNil bool) {
	defer func() { isNil = recover() == nil }()
	panic(nil)
}

func TestTry_panicNilUnderPanicnil1(t *testing.T) {
	if !recoverNil() {
		t.Fatal("//go:debug panicnil=1 not honoured: recover returned non-nil for panic(nil)")
	}
	err := panics.Try(func() error { panic(nil) })
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("Try = %v, want ErrPanic: panic(nil) is a panic even when recover returns nil", err)
	}
	var pe *panics.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("Try = %T, want *PanicError", err)
	}
	if pe.Value != nil {
		t.Errorf("Value = %v, want nil under panicnil=1", pe.Value)
	}
	if len(pe.Stack) == 0 {
		t.Error("Stack is empty")
	}
	var pne *runtime.PanicNilError
	if errors.As(err, &pne) {
		t.Error("found *runtime.PanicNilError; under panicnil=1 there is none")
	}
}

func TestTryValue_panicNilUnderPanicnil1(t *testing.T) {
	v, err := panics.TryValue(func() (int, error) { panic(nil) })
	if v != 0 || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("TryValue = %v, %v; want 0, ErrPanic", v, err)
	}
}
