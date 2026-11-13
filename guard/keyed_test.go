package guard_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/guard"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func ExampleKeyed() {
	var locks guard.Keyed[string] // the zero value is ready: one lock per account, made on first use
	ctx := context.Background()
	// One value per key: a Keyed serialises callers per key, so each key's value needs no other guard;
	// a single map shared by every key would still need its own Guarded (a map is one object).
	var alice, bob int

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			_ = locks.Do(ctx, "alice", func() error { alice++; return nil })
		})
		wg.Go(func() {
			_ = locks.Do(ctx, "bob", func() error { bob++; return nil })
		})
	}
	wg.Wait()
	fmt.Println("alice", alice, "bob", bob, "keys in use", locks.Len())

	// A held key makes TryDo report false; Do on it waits (bounded by its ctx).
	held, release := make(chan struct{}), make(chan struct{})
	wg.Go(func() {
		_ = locks.Do(ctx, "alice", func() error { close(held); <-release; return nil })
	})
	<-held
	ran, err := locks.TryDo("alice", func() error { return nil })
	fmt.Println("alice held: ran", ran, "err", err)
	close(release)
	wg.Wait()
	ran, err = locks.TryDo("alice", func() error { return nil })
	fmt.Println("alice free: ran", ran, "err", err)
	// Output:
	// alice 100 bob 100 keys in use 0
	// alice held: ran false err <nil>
	// alice free: ran true err <nil>
}

// hold takes key on its own goroutine and returns held (closed once fn is inside Do), release (close it to let
// the holder leave) and done (carries that Do's error once it has returned). The holder also leaves when ctx
// ends, so a failed assertion never strands it.
func hold[K comparable](t *testing.T, ctx context.Context, g *guard.Keyed[K], key K) (held <-chan struct{}, release chan<- struct{}, done <-chan error) {
	t.Helper()
	h, r, d := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		d <- g.Do(ctx, key, func() error {
			close(h)
			select {
			case <-r:
			case <-ctx.Done():
			}
			return nil
		})
	}()
	return h, r, d
}

// await receives from c or fails the test when ctx ends first.
func await[T any](t *testing.T, ctx context.Context, c <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s: %v", what, context.Cause(ctx))
		var zero T
		return zero
	}
}

// TestKeyed_goRatelimitOneLockAllKeys: distinct keys run at the same time. go-ratelimit
// per-client-rate-limit/main.go:51-52 takes one mutex for every client, so every client waits on every other;
// here two Do calls on different keys are both inside their fn at once, proved by each signalling before either
// is released. A regression (keys serialised) fails at the bounded wait instead of hanging.
func TestKeyed_goRatelimitOneLockAllKeys(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[string]

	heldA, releaseA, doneA := hold(t, ctx, &g, "a")
	heldB, releaseB, doneB := hold(t, ctx, &g, "b")

	await(t, ctx, heldA, "key a inside fn")
	await(t, ctx, heldB, "key b inside fn while a is held")
	if got := g.Len(); got != 2 {
		t.Fatalf("Len() = %d with two keys held, want 2", got)
	}
	close(releaseA)
	close(releaseB)
	if err := await(t, ctx, doneA, "Do a"); err != nil {
		t.Fatalf("Do(a) = %v, want nil", err)
	}
	if err := await(t, ctx, doneB, "Do b"); err != nil {
		t.Fatalf("Do(b) = %v, want nil", err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after both returned, want 0", got)
	}
}

// TestKeyed_sameKeySerialises: a plain counter incremented by 1000 goroutines under one key is exact (and -race
// is clean), so no two fn on the same key overlap.
func TestKeyed_sameKeySerialises(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const n = 1000
	var g guard.Keyed[int]
	var count int
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			if err := g.Do(ctx, 7, func() error { count++; return nil }); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Do: %v", err)
	}
	if count != n {
		t.Fatalf("count = %d, want %d", count, n)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after every Do returned, want 0", got)
	}
}

// TestKeyed_aDatabaseGetOrCreateMutexGrows: A-Database-Built-With-Golang main.go:173-185 keeps a mutex per
// collection in a map that is never emptied. Here 10 000 distinct keys pass through Do and the table is empty
// afterwards: entries are reference-counted and removed when the last holder leaves.
func TestKeyed_aDatabaseGetOrCreateMutexGrows(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	g := guard.NewKeyed[string]() // the default bound is DefaultMaxKeys: every key below is used and released
	const workers = 64            // bounded: the race detector allows 8128 live goroutines
	var wg sync.WaitGroup
	errs := make(chan error, guard.DefaultMaxKeys)
	for w := range workers {
		wg.Go(func() {
			for i := w; i < guard.DefaultMaxKeys; i += workers {
				key := "collection-" + strconv.Itoa(i)
				if err := g.Do(ctx, key, func() error { return nil }); err != nil {
					errs <- fmt.Errorf("%s: %w", key, err)
				}
				if ran, err := g.TryDo(key, func() error { return nil }); err != nil || !ran {
					errs <- fmt.Errorf("%s: TryDo = %v, %v", key, ran, err)
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after %d keys came and went, want 0", got, guard.DefaultMaxKeys)
	}
}

// TestKeyed_cancel: ctx done before Do returns the cause without an entry; ctx done while another holder holds
// the key returns the cause, leaves only the holder's entry, and the holder still unlocks cleanly; a cancel
// after Do returned changes nothing.
func TestKeyed_cancel(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[string]
	cause := errors.New("gave up")

	// Before.
	pre, cancelPre := context.WithCancelCause(ctx)
	cancelPre(cause)
	ran := false
	if err := g.Do(pre, "k", func() error { ran = true; return nil }); !errors.Is(err, cause) {
		t.Fatalf("Do with a done ctx = %v, want the cause %v", err, cause)
	}
	if ran {
		t.Fatal("fn ran although ctx was done before Do")
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after a refused Do, want 0", got)
	}

	// During: a holder has the key; a waiter is cancelled.
	held, release, holderDone := hold(t, ctx, &g, "k")
	await(t, ctx, held, "holder inside fn")
	waitCtx, cancelWait := context.WithCancelCause(ctx)
	waiterDone := make(chan error, 1)
	go func() {
		waiterDone <- g.Do(waitCtx, "k", func() error { t.Error("waiter's fn ran while the key was held"); return nil })
	}()
	cancelWait(cause)
	if err := await(t, ctx, waiterDone, "cancelled waiter"); !errors.Is(err, cause) {
		t.Fatalf("waiter Do = %v, want the cause %v", err, cause)
	}
	if got := g.Len(); got != 1 {
		t.Fatalf("Len() = %d with the holder still inside and the waiter gone, want 1", got)
	}
	close(release)
	if err := await(t, ctx, holderDone, "holder"); err != nil {
		t.Fatalf("holder Do = %v, want nil", err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after the holder left, want 0", got)
	}
	if ran, err := g.TryDo("k", func() error { return nil }); !ran || err != nil {
		t.Fatalf("TryDo after the holder left = %v, %v; want true, nil (the key is free)", ran, err)
	}

	// After.
	after, cancelAfter := context.WithCancelCause(ctx)
	if err := g.Do(after, "k", func() error { return nil }); err != nil {
		t.Fatalf("Do = %v, want nil", err)
	}
	cancelAfter(cause)
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after a cancel following a completed Do, want 0", got)
	}
}

func TestKeyed_TryDo(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[string]

	calls := 0
	if ran, err := g.TryDo("k", func() error { calls++; return nil }); !ran || err != nil || calls != 1 {
		t.Fatalf("TryDo on a free key = %v, %v (calls %d); want true, nil, 1", ran, err, calls)
	}

	held, release, done := hold(t, ctx, &g, "k")
	await(t, ctx, held, "holder inside fn")
	if ran, err := g.TryDo("k", func() error { calls++; return nil }); ran || err != nil || calls != 1 {
		t.Fatalf("TryDo on a held key = %v, %v (calls %d); want false, nil, 1 (fn not run)", ran, err, calls)
	}
	if ran, err := g.TryDo("other", func() error { calls++; return nil }); !ran || err != nil || calls != 2 {
		t.Fatalf("TryDo on another key while k is held = %v, %v (calls %d); want true, nil, 2", ran, err, calls)
	}
	if got := g.Len(); got != 1 {
		t.Fatalf("Len() = %d with one holder and a failed TryDo gone, want 1", got)
	}
	close(release)
	if err := await(t, ctx, done, "holder"); err != nil {
		t.Fatalf("holder Do = %v, want nil", err)
	}
	if ran, err := g.TryDo("k", func() error { calls++; return nil }); !ran || err != nil || calls != 3 {
		t.Fatalf("TryDo after release = %v, %v (calls %d); want true, nil, 3", ran, err, calls)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d when idle, want 0", got)
	}
}

// TestKeyed_ErrKeys: with WithMaxKeys(2) and two keys held, a third key is refused with ErrKeys carrying the
// count; a key already in the table is never refused; once a holder leaves the third key is admitted.
func TestKeyed_ErrKeys(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	g := guard.NewKeyed[string](guard.WithMaxKeys(2), nil) // a nil option is ignored

	heldA, releaseA, doneA := hold(t, ctx, g, "a")
	heldB, releaseB, doneB := hold(t, ctx, g, "b")
	await(t, ctx, heldA, "a held")
	await(t, ctx, heldB, "b held")

	err := g.Do(ctx, "c", func() error { t.Error("fn ran on a refused key"); return nil })
	if !errors.Is(err, guard.ErrKeys) {
		t.Fatalf("Do on a third key = %v, want ErrKeys", err)
	}
	if !strings.Contains(err.Error(), "2") {
		t.Fatalf("ErrKeys message %q does not carry the count 2", err)
	}
	if ran, err := g.TryDo("c", func() error { return nil }); ran || !errors.Is(err, guard.ErrKeys) {
		t.Fatalf("TryDo on a third key = %v, %v; want false, ErrKeys", ran, err)
	}
	if ran, err := g.TryDo("a", func() error { return nil }); ran || err != nil {
		t.Fatalf("TryDo on a held key in a full table = %v, %v; want false, nil (held, not refused)", ran, err)
	}
	if got := g.Len(); got != 2 {
		t.Fatalf("Len() = %d after refusals, want 2 (refused keys leave no entry)", got)
	}

	close(releaseA)
	if err := await(t, ctx, doneA, "holder a"); err != nil {
		t.Fatalf("Do(a) = %v, want nil", err)
	}
	if err := g.Do(ctx, "c", func() error { return nil }); err != nil {
		t.Fatalf("Do on c after a left = %v, want nil (admitted again)", err)
	}
	close(releaseB)
	if err := await(t, ctx, doneB, "holder b"); err != nil {
		t.Fatalf("Do(b) = %v, want nil", err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d when idle, want 0", got)
	}
}

// TestKeyed_panicInFn: a panic in fn comes back as ErrPanic from Do and from TryDo, and the key is free afterwards.
func TestKeyed_panicInFn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[string]

	err := g.Do(ctx, "k", func() error { panic("boom") })
	var pe *panics.PanicError
	if !errors.Is(err, panics.ErrPanic) || !errors.As(err, &pe) {
		t.Fatalf("Do with a panicking fn = %v, want a *panics.PanicError matching ErrPanic", err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after a panic, want 0 (the key was released)", got)
	}
	ran, err := g.TryDo("k", func() error { panic("boom again") })
	if !ran || !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("TryDo with a panicking fn = %v, %v; want true (it ran), ErrPanic", ran, err)
	}
	if err := g.Do(ctx, "k", func() error { return nil }); err != nil {
		t.Fatalf("Do after two panics = %v, want nil (the key is free)", err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d when idle, want 0", got)
	}
}

// TestKeyed_fnError: fn's own error is returned unchanged and the key is released.
func TestKeyed_fnError(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[string]
	want := errors.New("fn failed")
	if err := g.Do(ctx, "k", func() error { return want }); err != want {
		t.Fatalf("Do = %v, want fn's error %v unchanged", err, want)
	}
	if ran, err := g.TryDo("k", func() error { return want }); !ran || err != want {
		t.Fatalf("TryDo = %v, %v; want true, fn's error %v unchanged", ran, err, want)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d when idle, want 0", got)
	}
}

func TestKeyed_nilFn(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[string]
	if err := g.Do(ctx, "k", nil); err != nil {
		t.Fatalf("Do(nil fn) = %v, want nil", err)
	}
	if ran, err := g.TryDo("k", nil); !ran || err != nil {
		t.Fatalf("TryDo(nil fn) = %v, %v; want true, nil", ran, err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d after nil fns, want 0 (the table is not touched)", got)
	}
}

func TestKeyed_zeroValue(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[int]
	if got := g.Len(); got != 0 {
		t.Fatalf("zero Keyed Len() = %d, want 0", got)
	}
	ran := false
	if err := g.Do(ctx, 1, func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("zero Keyed Do = %v (ran %v), want nil, true", err, ran)
	}
	n := guard.NewKeyed[int](nil)
	if err := n.Do(ctx, 1, func() error { return nil }); err != nil {
		t.Fatalf("NewKeyed(nil) Do = %v, want nil", err)
	}
	if got := g.Len() + n.Len(); got != 0 {
		t.Fatalf("Len() sum = %d when idle, want 0", got)
	}
}

func TestKeyed_nilCtx(t *testing.T) {
	leak.Verify(t)
	var g guard.Keyed[string]
	ran := false
	//lint:ignore SA1012 a nil ctx is documented as context.Background()
	err := g.Do(nil, "k", func() error { ran = true; return nil }) //nolint:staticcheck
	if err != nil || !ran {
		t.Fatalf("Do(nil ctx) = %v (ran %v), want nil, true", err, ran)
	}
}

// TestKeyed_keyNotEqualToItself: a NaN key can never be deleted from a map, so it is refused with ErrKeys and
// leaves no entry.
func TestKeyed_keyNotEqualToItself(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[float64]
	if err := g.Do(ctx, math.NaN(), func() error { t.Error("fn ran on a NaN key"); return nil }); !errors.Is(err, guard.ErrKeys) {
		t.Fatalf("Do(NaN) = %v, want ErrKeys", err)
	}
	if ran, err := g.TryDo(math.NaN(), func() error { return nil }); ran || !errors.Is(err, guard.ErrKeys) {
		t.Fatalf("TryDo(NaN) = %v, %v; want false, ErrKeys", ran, err)
	}
	if err := g.Do(ctx, 1.5, func() error { return nil }); err != nil {
		t.Fatalf("Do(1.5) = %v, want nil", err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d, want 0", got)
	}
}

// TestKeyed_WithMaxKeysNonPositive: n <= 0 means the default, so more than n keys are admitted.
func TestKeyed_WithMaxKeysNonPositive(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for _, n := range []int{0, -1} {
		g := guard.NewKeyed[int](guard.WithMaxKeys(n))
		heldA, releaseA, doneA := hold(t, ctx, g, 1)
		heldB, releaseB, doneB := hold(t, ctx, g, 2)
		await(t, ctx, heldA, "key 1 held")
		await(t, ctx, heldB, "key 2 held")
		if err := g.Do(ctx, 3, func() error { return nil }); err != nil {
			t.Fatalf("WithMaxKeys(%d): Do on a third key = %v, want nil (default bound)", n, err)
		}
		close(releaseA)
		close(releaseB)
		if err := errors.Join(await(t, ctx, doneA, "holder 1"), await(t, ctx, doneB, "holder 2")); err != nil {
			t.Fatalf("holders: %v", err)
		}
	}
}

func BenchmarkKeyed_Do(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()
	var g guard.Keyed[string]
	noop := func() error { return nil }
	for b.Loop() {
		if err := g.Do(ctx, "hot", noop); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKeyed_DoDistinctKeys(b *testing.B) {
	b.ReportAllocs()
	ctx := context.Background()
	var g guard.Keyed[int]
	noop := func() error { return nil }
	b.RunParallel(func(pb *testing.PB) {
		key := 0
		for pb.Next() {
			key = (key + 1) % 64
			if err := g.Do(ctx, key, noop); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func ExampleNewKeyed() {
	locks := guard.NewKeyed[string](guard.WithMaxKeys(100)) // options only; var locks guard.Keyed[string] works too
	err := locks.Do(context.Background(), "order-42", func() error {
		fmt.Println("inside order-42, keys in use:", locks.Len())
		return nil
	})
	fmt.Println("err", err, "keys in use after:", locks.Len())
	// Output:
	// inside order-42, keys in use: 1
	// err <nil> keys in use after: 0
}

// ExampleWithMaxKeys: WithMaxKeys is a KeyedOption; without it the bound is DefaultMaxKeys (10 000). A new key
// that finds the bound full is refused with ErrKeys; a key already in the table is not.
func ExampleWithMaxKeys() {
	ctx := context.Background()
	locks := guard.NewKeyed[string](guard.WithMaxKeys(1))
	fmt.Println("default bound:", guard.DefaultMaxKeys)

	held, release := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		_ = locks.Do(ctx, "a", func() error { close(held); <-release; return nil })
	})
	<-held
	err := locks.Do(ctx, "b", func() error { return nil })
	fmt.Println("b while a is held:", err, "| is ErrKeys:", errors.Is(err, guard.ErrKeys))
	close(release)
	wg.Wait()
	fmt.Println("b after a left:", locks.Do(ctx, "b", func() error { return nil }))
	// Output:
	// default bound: 10000
	// b while a is held: guard: key bound full: 1 keys held or waited on | is ErrKeys: true
	// b after a left: <nil>
}

// ExampleErrKeys: ErrKeys is always wrapped; match it with errors.Is. A key that is not equal to itself (NaN)
// could never be deleted from the table, so it is refused.
func ExampleErrKeys() {
	var locks guard.Keyed[float64]
	err := locks.Do(context.Background(), math.NaN(), func() error { return nil })
	fmt.Println(errors.Is(err, guard.ErrKeys), "|", err)
	// Output:
	// true | guard: key bound full: key is not equal to itself
}

// TestKeyed_uncomparableKeyIsErrKeys: a Keyed[any] given a []int key. Comparing or hashing it panics in Go;
// here Do and TryDo return ErrKeys instead, the table mutex is not left locked (Len still answers), and a later
// Do on a good key works.
func TestKeyed_uncomparableKeyIsErrKeys(t *testing.T) {
	leak.Verify(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var g guard.Keyed[any]
	bad := any([]int{1, 2})
	if err := g.Do(ctx, bad, func() error { t.Error("fn ran on an uncomparable key"); return nil }); !errors.Is(err, guard.ErrKeys) {
		t.Fatalf("Do([]int key) = %v, want ErrKeys", err)
	}
	if ran, err := g.TryDo(bad, func() error { return nil }); ran || !errors.Is(err, guard.ErrKeys) {
		t.Fatalf("TryDo([]int key) = %v, %v; want false, ErrKeys", ran, err)
	}
	lenDone := make(chan int, 1)
	go func() { lenDone <- g.Len() }() // would hang if the table mutex were left locked
	if got := await(t, ctx, lenDone, "Len after refused keys"); got != 0 {
		t.Fatalf("Len() = %d after refused keys, want 0", got)
	}
	ran := false
	if err := g.Do(ctx, "good", func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("Do(good key) = %v (ran %v), want nil, true", err, ran)
	}
	if ran, err := g.TryDo(7, func() error { return nil }); !ran || err != nil {
		t.Fatalf("TryDo(7) = %v, %v; want true, nil", ran, err)
	}
	if got := g.Len(); got != 0 {
		t.Fatalf("Len() = %d when idle, want 0", got)
	}
}
