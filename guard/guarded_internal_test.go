package guard

import "testing"

// TestGuarded_DoPanicUnlocks: a panic in fn reaches the caller unchanged, the value keeps what fn wrote before
// the panic, and the mutex is unlocked (checked with TryLock, so a regression fails instead of hanging).
func TestGuarded_DoPanicUnlocks(t *testing.T) {
	var g Guarded[int]
	got := func() (r any) {
		defer func() { r = recover() }()
		g.Do(func(v *int) {
			*v = 1
			panic("boom")
		})
		return nil
	}()
	if got != "boom" {
		t.Fatalf("recovered %v, want the panic value %q to propagate from Do", got, "boom")
	}
	if !g.mu.TryLock() {
		t.Fatal("mutex still locked after a panic in Do")
	}
	g.mu.Unlock()
	if v := g.Load(); v != 1 {
		t.Fatalf("Load() = %d, want 1 (the write made before the panic)", v)
	}
}
