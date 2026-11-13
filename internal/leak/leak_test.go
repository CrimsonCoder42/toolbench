package leak_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

// The package checks itself: every goroutine a test below starts has exited when the tests end.
func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// blockedOn signals started, parks until release is closed, then signals done. A named function, so its frame
// is findable in the stack.
func blockedOn(started chan<- struct{}, release <-chan struct{}, done chan<- struct{}) {
	close(started)
	<-release
	close(done)
}

// park starts blockedOn and returns once it is running, with the function that releases it and waits for it.
func park() (stop func()) {
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go blockedOn(started, release, done)
	<-started
	return func() { close(release); <-done }
}

// recorder is a testing.TB that records Errorf instead of failing the real test. Only Helper and Errorf
// are called by the package; any other method would panic on the nil embedded TB.
type recorder struct {
	testing.TB
	failed bool
	msg    string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.failed = true
	r.msg = fmt.Sprintf(format, args...)
}

func TestTake_includesCaller(t *testing.T) {
	s := leak.Take()
	if extra := s.Extra(leak.WithTimeout(0)); len(extra) != 0 {
		t.Fatalf("a snapshot checked against itself has %d extra goroutines:\n%v", len(extra), extra)
	}
}

func TestExtra_noneAfterGoroutineExits(t *testing.T) {
	s := leak.Take()
	park()()
	if extra := s.Extra(); len(extra) != 0 {
		t.Fatalf("extra after the goroutine exited: %v", extra)
	}
}

func TestExtra_settlesGoroutineThatIsExiting(t *testing.T) {
	s := leak.Take()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go blockedOn(started, release, done)
	<-started
	close(release) // the goroutine may still be alive at the first look; the settle loop waits for it
	if extra := s.Extra(); len(extra) != 0 {
		t.Fatalf("extra after settling: %v", extra)
	}
	<-done
}

func TestExtra_reportsLeakWithStack(t *testing.T) {
	s := leak.Take()
	defer park()()

	extra := s.Extra(leak.WithTimeout(20 * time.Millisecond))
	if len(extra) != 1 {
		t.Fatalf("got %d extra goroutines, want 1: %v", len(extra), extra)
	}
	g := extra[0]
	if !strings.HasSuffix(g.Top, ".blockedOn") {
		t.Errorf("Top = %q, want the blockedOn frame", g.Top)
	}
	if g.State != "chan receive" {
		t.Errorf("State = %q, want %q", g.State, "chan receive")
	}
	if !strings.Contains(g.Stack, "leak_test.go:") || g.ID == 0 {
		t.Errorf("stack lacks the file:line or id: %+v", g)
	}
}

func TestExtra_zeroSnapshotCountsEveryGoroutine(t *testing.T) {
	var s leak.Snapshot
	if extra := s.Extra(leak.WithTimeout(0)); len(extra) == 0 {
		t.Fatal("the zero Snapshot reported no extra goroutines; the caller itself should count")
	}
}

func TestWithIgnoreTopFunction_skipsMatchingGoroutine(t *testing.T) {
	s := leak.Take()
	defer park()()

	extra := s.Extra(leak.WithTimeout(0))
	if len(extra) != 1 {
		t.Fatalf("got %d extra goroutines before ignoring, want 1: %v", len(extra), extra)
	}
	if got := s.Extra(leak.WithTimeout(0), leak.WithIgnoreTopFunction(extra[0].Top)); len(got) != 0 {
		t.Fatalf("ignored top function %q still reported: %v", extra[0].Top, got)
	}
}

func TestCheck_failsWithStacks(t *testing.T) {
	s := leak.Take()
	defer park()()

	r := &recorder{}
	leak.Check(r, s, leak.WithTimeout(10*time.Millisecond))
	if !r.failed {
		t.Fatal("Check did not fail on a leaked goroutine")
	}
	if !strings.Contains(r.msg, "1 goroutine(s) still running") || !strings.Contains(r.msg, "blockedOn") {
		t.Errorf("message lacks the count or the stack:\n%s", r.msg)
	}
}

func TestCheck_passesWhenClean(t *testing.T) {
	s := leak.Take()
	r := &recorder{}
	leak.Check(r, s)
	if r.failed {
		t.Fatalf("Check failed with nothing leaked:\n%s", r.msg)
	}
}

func TestVerify_checksAtCleanup(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		leak.Verify(t)
		park()()
	})
}

func ExampleSnapshot_Extra() {
	before := leak.Take()
	done := make(chan struct{})
	go func() { close(done) }()
	<-done
	fmt.Println(len(before.Extra()))
	// Output: 0
}
