// Package leak is the goroutine-leak check every toolbench test uses (.claude/rules/go-testing.md, rule 5):
// take a snapshot of the live goroutines before, let the goroutines a test started settle for a bounded time
// after, and fail with the stack of every goroutine that is still alive and was not there before.
//
// It is test tooling, not a pattern, and carries no ID. It uses only the standard library (runtime,
// runtime/pprof). The settle loop waits on a timer channel and never calls time.Sleep.
//
// Usage, once per package:
//
//	func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }
//
// or per test (not with t.Parallel, whose sibling goroutines would count as extra):
//
//	func TestX(t *testing.T) { leak.Verify(t); ... }
package leak

import (
	"bytes"
	"cmp"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// DefaultTimeout is how long Extra waits for extra goroutines to exit before reporting them.
const DefaultTimeout = 5 * time.Second

// defaultIgnore are top functions of goroutines that belong to the test binary itself, not to the code under
// test: the testing package's own runners and the os/signal loop, which lives for the life of the process.
var defaultIgnore = []string{"testing.", "os/signal.signal_recv", "os/signal.loop"}

// Goroutine is one live goroutine as the runtime reports it.
type Goroutine struct {
	ID    uint64 // the runtime's goroutine id
	State string // the wait reason, for example "chan receive" or "select"
	Top   string // the innermost function shown in the stack, without arguments
	Stack string // the full stack as runtime/pprof prints it (debug=2)
}

func (g Goroutine) String() string { return g.Stack }

// Snapshot is the set of goroutines alive at one moment. Create it with Take; the zero value is an empty set,
// so every live goroutine counts as extra against it.
type Snapshot struct {
	ids map[uint64]bool
}

// Option changes how Extra, Check, Verify and Main settle and filter.
type Option func(*config)

type config struct {
	timeout time.Duration
	ignore  []string
}

// WithTimeout sets how long to wait for extra goroutines to exit (default DefaultTimeout). A non-positive d
// checks once, without waiting.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithIgnoreTopFunction ignores every goroutine whose innermost function starts with prefix, for a goroutine
// the code under test deliberately leaves running for the life of the process.
func WithIgnoreTopFunction(prefix string) Option {
	return func(c *config) { c.ignore = append(c.ignore, prefix) }
}

func newConfig(opts []Option) config {
	c := config{timeout: DefaultTimeout, ignore: slices.Clone(defaultIgnore)}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// Take records the goroutines alive now, the calling goroutine included.
func Take() Snapshot {
	s := Snapshot{ids: map[uint64]bool{}}
	for _, g := range current() {
		s.ids[g.ID] = true
	}
	return s
}

// Extra returns the goroutines alive now that were not in s and are not ignored. While there are any, it
// re-checks on a timer channel until they exit or the timeout passes, so a goroutine that is finishing as the
// test ends is not reported. The result is sorted by goroutine id and is empty when nothing leaked.
func (s Snapshot) Extra(opts ...Option) []Goroutine {
	c := newConfig(opts)
	extra := s.extra(c)
	if len(extra) == 0 || c.timeout <= 0 {
		return extra
	}
	deadline := time.NewTimer(c.timeout)
	defer deadline.Stop()
	wait := time.Millisecond
	for {
		retry := time.NewTimer(wait)
		select {
		case <-deadline.C:
			retry.Stop()
			return s.extra(c)
		case <-retry.C:
		}
		runtime.Gosched()
		if extra = s.extra(c); len(extra) == 0 {
			return extra
		}
		wait = min(2*wait, 100*time.Millisecond)
	}
}

func (s Snapshot) extra(c config) []Goroutine {
	var out []Goroutine
	for _, g := range current() {
		if s.ids[g.ID] || ignored(g.Top, c.ignore) {
			continue
		}
		out = append(out, g)
	}
	return out
}

func ignored(top string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(top, p) {
			return true
		}
	}
	return false
}

// Check fails t with the stack of every goroutine that is alive, was not in before and does not exit within
// the timeout.
func Check(t testing.TB, before Snapshot, opts ...Option) {
	t.Helper()
	if extra := before.Extra(opts...); len(extra) > 0 {
		t.Errorf("%s", report(extra))
	}
}

// Verify takes a snapshot now and checks it when t and its subtests finish (t.Cleanup). It is not for a test
// that calls t.Parallel.
func Verify(t testing.TB, opts ...Option) {
	t.Helper()
	before := Take()
	t.Cleanup(func() { Check(t, before, opts...) })
}

// Main runs the package's tests and then checks that every goroutine they started has exited. It returns the
// exit code for os.Exit: m.Run's code when that is non-zero, 1 with the leaked stacks on stderr when a
// goroutine leaked, else 0.
func Main(m *testing.M, opts ...Option) int {
	before := Take()
	if code := m.Run(); code != 0 {
		return code
	}
	if extra := before.Extra(opts...); len(extra) > 0 {
		fmt.Fprintln(os.Stderr, report(extra))
		return 1
	}
	return 0
}

func report(extra []Goroutine) string {
	var b strings.Builder
	fmt.Fprintf(&b, "leak: %d goroutine(s) still running:", len(extra))
	for _, g := range extra {
		b.WriteString("\n\n")
		b.WriteString(g.Stack)
	}
	return b.String()
}

var headerRe = regexp.MustCompile(`^goroutine (\d+)\b[^\[]*\[([^\]]*)\]`)

// current returns every live goroutine, parsed from the runtime/pprof goroutine profile at debug=2 (the same
// text a panic prints), sorted by id.
func current() []Goroutine {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 2); err != nil {
		// Writing to a bytes.Buffer does not fail; if it ever does, an empty list would pass every check.
		panic("leak: goroutine profile: " + err.Error())
	}
	var out []Goroutine
	for block := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n\n") {
		lines := strings.Split(block, "\n")
		m := headerRe.FindStringSubmatch(lines[0])
		if m == nil {
			continue
		}
		id, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		g := Goroutine{ID: id, State: m[2], Stack: block}
		if len(lines) > 1 {
			g.Top = funcName(lines[1])
		}
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b Goroutine) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// funcName strips the argument list from a stack frame line: "pkg.(*T).M(0x1, ...)" becomes "pkg.(*T).M".
func funcName(frame string) string {
	frame = strings.TrimSpace(frame)
	if i := strings.LastIndex(frame, "("); i > 0 && strings.HasSuffix(frame, ")") {
		return frame[:i]
	}
	return frame
}
