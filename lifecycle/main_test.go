package lifecycle_test

import (
	"context"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/lifecycle"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// bounded is the test's own deadline: every wait in a test also selects on it.
func bounded(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// recorder keeps the order of starts and stops across goroutines.
type recorder struct {
	mu  sync.Mutex
	log []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, s)
}

func (r *recorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.log)
}

// comp is a component that records its start and stop.
func comp(r *recorder, name string) lifecycle.Component {
	return lifecycle.Component{
		Name:  name,
		Start: func(context.Context) error { r.add("start " + name); return nil },
		Stop:  func(context.Context) error { r.add("stop " + name); return nil },
	}
}

// trigger is a component, added last, whose Start cancels the run: every component before it is started.
func trigger(cancel context.CancelFunc) lifecycle.Component {
	return lifecycle.Component{Name: "trigger", Start: func(context.Context) error { cancel(); return nil }}
}

// runAsync runs g.Run on a goroutine and returns the channel of its result.
func runAsync(ctx context.Context, g *lifecycle.Group, opts ...lifecycle.Option) <-chan error {
	done := make(chan error, 1)
	go func() { done <- g.Run(ctx, opts...) }()
	return done
}

// await receives Run's result or fails the test at its deadline.
func await(t *testing.T, ctx context.Context, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatalf("Run did not return: %v", context.Cause(ctx))
		return nil
	}
}
