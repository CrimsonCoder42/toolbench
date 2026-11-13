package logging_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/logging"
)

// houseTime is the fixed clock's time and ts its house rendering (log.Ldate|Ltime|Lmicroseconds).
var houseTime = time.Date(2026, 11, 13, 9, 4, 5, 678901000, time.UTC)

const ts = "2026/11/13 09:04:05.678901"

// textLogger is a house Text logger on buf at Debug with the fixed clock, plus opts.
func textLogger(buf *bytes.Buffer, opts ...logging.TextOption) *slog.Logger {
	all := append([]logging.TextOption{logging.WithClock(clock.NewFake(houseTime)), logging.WithLevel(slog.LevelDebug)}, opts...)
	return slog.New(logging.Text(buf, all...))
}

// here returns "file.go:line: " for the line after the caller's, the S2 source the next log call prints.
func here() string {
	_, file, line, _ := runtime.Caller(1)
	return fmt.Sprintf("%s:%d: ", filepath.Base(file), line+1)
}

// golden compares exact bytes; the output under test carries no secret, so it is printed with %q on failure.
func golden(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("\n got %q\nwant %q", got, want)
	}
}

// myErr's Error reads its receiver, so a nil *myErr inside an error prints "<nil>" through fmt.
type myErr struct{ msg string }

func (e *myErr) Error() string { return e.msg }

// S1: the plain line shape, timestamp, no module, no padding.
func TestText_S1_PlainLine(t *testing.T) {
	var buf bytes.Buffer
	textLogger(&buf).Info("server started")
	golden(t, buf.String(), "INFO: "+ts+" server started\n")
}

// S1: the module attribute prints "MODULE: " before the message, as given; attrs in written order.
func TestText_S1_ModuleAndAttrs(t *testing.T) {
	var buf bytes.Buffer
	textLogger(&buf).Info("ListAuthUsers SUCCESS", "module", "AUTH_USER", "tenant_id", "e1c6acea", "active", 0, "pending", 0)
	golden(t, buf.String(), "INFO: "+ts+" AUTH_USER: ListAuthUsers SUCCESS | tenant_id=e1c6acea | active=0 | pending=0\n")
}

// S1: the module keeps its case (never upper-cased), comes from With, and a record module wins over it.
func TestText_S1_ModuleCaseWithAndOverride(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf).With("module", "Feed", "tenant_id", "t1")
	l.Info("a")
	l.Info("b", "module", "CART")
	golden(t, buf.String(), "INFO: "+ts+" Feed: a | tenant_id=t1\nINFO: "+ts+" CART: b | tenant_id=t1\n")
}

// S1: a module is never invented; an empty or nil module prints none.
func TestText_S1_ModuleNeverInvented(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf)
	l.Info("m", "n", 1)
	l.Info("m", "module", "")
	l.Info("m", "module", nil)
	golden(t, buf.String(), "INFO: "+ts+" m | n=1\nINFO: "+ts+" m\nINFO: "+ts+" m\n")
}

// S1: with no module attribute the logger's outermost group is the module; its keys still flatten.
func TestText_S1_ModuleFromGroup(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf).WithGroup("cache").WithGroup("lru")
	l.Info("hit", "n", 2)
	textLogger(&buf).With("module", "AUTH").WithGroup("cache").Info("hit", "n", 3)
	golden(t, buf.String(), "INFO: "+ts+" cache: hit | cache.lru.n=2\nINFO: "+ts+" AUTH: hit | cache.n=3\n")
}

// S1: missing, nil and empty values print absent.
func TestText_S1_Absent(t *testing.T) {
	var buf bytes.Buffer
	var nilErr *myErr
	textLogger(&buf).Info("m", "a", nil, "b", "", "c", error(nil), "d", nilErr, "f", 0)
	golden(t, buf.String(), "INFO: "+ts+" m | a=absent | b=absent | c=absent | d=absent | f=0\n")
}

// S1: groups flatten to group.key at any depth; empty groups vanish; an empty-key group is inlined.
func TestText_S1_GroupsFlatten(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf).With("a", 1).WithGroup("g").With("b", 2)
	l.Info("m", slog.Group("h", "c", 3, slog.Group("i", "d", 4)), slog.Group("empty"), slog.Group("", "e", 5))
	golden(t, buf.String(), "INFO: "+ts+" g: m | a=1 | g.b=2 | g.h.c=3 | g.h.i.d=4 | g.e=5\n")
}

// S1: value kinds print as fmt's %v.
func TestText_S1_ValueKinds(t *testing.T) {
	var buf bytes.Buffer
	textLogger(&buf).Info("m", "i", -3, "u", uint64(7), "f", 1.5, "b", true, "d", 1500*time.Millisecond,
		"t", houseTime, "s", []int{1, 2})
	golden(t, buf.String(), "INFO: "+ts+" m | i=-3 | u=7 | f=1.5 | b=true | d=1.5s | t=2026-11-13 09:04:05.678901 +0000 UTC | s=[1 2]\n")
}

// S1: every level label, unpadded; a level between names uses slog's name.
func TestText_S1_LevelLabels(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf, logging.WithSource(slog.LevelError+4))
	ctx := context.Background()
	l.Debug("d")
	l.Info("i")
	l.Warn("w")
	l.Error("e")
	l.Log(ctx, slog.LevelInfo+2, "x")
	golden(t, buf.String(), "DEBUG: "+ts+" d\nINFO: "+ts+" i\nWARN: "+ts+" w\nERROR: "+ts+" e\nINFO+2: "+ts+" x\n")
}

// S1: WithTimeFormat changes or removes the timestamp; a zero record time with no clock prints none.
func TestText_S1_TimeFormatAndZeroTime(t *testing.T) {
	var buf bytes.Buffer
	textLogger(&buf, logging.WithTimeFormat(time.RFC3339)).Info("a")
	textLogger(&buf, logging.WithTimeFormat("")).Info("b")
	h := logging.Text(&buf)
	if err := h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "c", 0)); err != nil {
		t.Fatal(err)
	}
	if err := h.Handle(context.Background(), slog.NewRecord(houseTime, slog.LevelInfo, "d", 0)); err != nil {
		t.Fatal(err)
	}
	golden(t, buf.String(), "INFO: 2026-11-13T09:04:05Z a\nINFO: b\nINFO: c\nINFO: "+ts+" d\n")
}

// S1: the level threshold, default Info; a LevelVar changes it.
func TestText_S1_Level(t *testing.T) {
	var buf bytes.Buffer
	var lv slog.LevelVar
	lv.Set(slog.LevelWarn)
	l := slog.New(logging.Text(&buf, logging.WithClock(clock.NewFake(houseTime)), logging.WithLevel(&lv)))
	l.Info("hidden")
	lv.Set(slog.LevelInfo)
	l.Info("shown")
	l.Debug("hidden")
	golden(t, buf.String(), "INFO: "+ts+" shown\n")
	if logging.Text(&buf).Enabled(context.Background(), slog.LevelDebug) {
		t.Error("default level enables Debug")
	}
}

// S2: source after the timestamp on WARN and ERROR only, before the module.
func TestText_S2_SourceOnWarnAndErrorOnly(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf)
	l.Debug("d")
	l.Info("i")
	w := here()
	l.Warn("w", "module", "FEED")
	e := here()
	l.Error("e")
	golden(t, buf.String(), "DEBUG: "+ts+" d\nINFO: "+ts+" i\nWARN: "+ts+" "+w+"FEED: w\nERROR: "+ts+" "+e+"e\n")
}

// S2: WithSource moves the threshold; a record with a zero PC carries no source.
func TestText_S2_WithSourceAndZeroPC(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf, logging.WithSource(slog.LevelInfo))
	i := here()
	l.Info("i")
	h := logging.Text(&buf, logging.WithClock(clock.NewFake(houseTime)))
	if err := h.Handle(context.Background(), slog.NewRecord(houseTime, slog.LevelError, "e", 0)); err != nil {
		t.Fatal(err)
	}
	golden(t, buf.String(), "INFO: "+ts+" "+i+"i\nERROR: "+ts+" e\n")
}

// S2: err renders last, With errs before record errs; a nil err is absent; err inside a group stays put.
func TestText_S2_ErrLast(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf, logging.WithSource(slog.LevelError+4))
	l.Error("drug names unavailable", "module", "FEED", "err", errors.New("upstream timeout"), "n", 2)
	l.With("err", errors.New("first")).Error("m", "err", errors.New("second"), "k", "v")
	l.Error("m", "err", nil)
	l.WithGroup("g").Error("m", "err", errors.New("inner"), "k", 1)
	golden(t, buf.String(),
		"ERROR: "+ts+" FEED: drug names unavailable | n=2 | err=upstream timeout\n"+
			"ERROR: "+ts+" m | k=v | err=first | err=second\n"+
			"ERROR: "+ts+" m | err=absent\n"+
			"ERROR: "+ts+" g: m | g.err=inner | g.k=1\n")
}

// S3: colour on the level prefix only, util/logger.go's codes, only with WithColor(true).
func TestText_S3_Color(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf, logging.WithColor(true), logging.WithSource(slog.LevelError+4))
	l.Debug("d")
	l.Info("i", "k", "v")
	l.Warn("w")
	l.Error("e")
	golden(t, buf.String(),
		"DEBUG: "+ts+" d\n"+
			"\033[34mINFO: \033[0m"+ts+" i | k=v\n"+
			"\033[33mWARN: \033[0m"+ts+" w\n"+
			"\033[31mERROR: \033[0m"+ts+" e\n")
}

// S3: no escape byte is written by default or with WithColor(false).
func TestText_S3_NoColorByDefault(t *testing.T) {
	var buf bytes.Buffer
	for _, l := range []*slog.Logger{textLogger(&buf), textLogger(&buf, logging.WithColor(false))} {
		l.Debug("d")
		l.Info("i")
		l.Warn("w")
		l.Error("e")
	}
	if strings.ContainsRune(buf.String(), '\033') {
		t.Errorf("escape byte in %q", buf.String())
	}
	if got := len(lines(buf.String())); got != 8 {
		t.Errorf("lines = %d, want 8", got)
	}
}

// S1: values are resolved (LogValuer) in records and With.
func TestText_S1_ResolvesLogValuer(t *testing.T) {
	var buf bytes.Buffer
	n := 0
	textLogger(&buf).With("w", lazy{&n}).Info("m", "r", lazy{&n})
	golden(t, buf.String(), "INFO: "+ts+" m | w=resolved | r=resolved\n")
}

// WithAttrs and WithGroup never change the receiver.
func TestText_DerivedHandlersIndependent(t *testing.T) {
	var buf bytes.Buffer
	base := textLogger(&buf)
	a := base.With("a", 1)
	_ = a.With("module", "X", "err", errors.New("e")).WithGroup("g").With("c", 3)
	_ = base.WithGroup("h")
	a.Info("m")
	base.Info("m")
	golden(t, buf.String(), "INFO: "+ts+" m | a=1\nINFO: "+ts+" m\n")
}

// A nil writer is io.Discard; Write errors are returned.
func TestText_NilWriterAndWriteError(t *testing.T) {
	ctx := context.Background()
	if err := logging.Text(nil).Handle(ctx, slog.NewRecord(houseTime, slog.LevelInfo, "m", 0)); err != nil {
		t.Fatalf("nil writer: %v", err)
	}
	want := errors.New("disk full")
	err := logging.Text(failWriter{want}).Handle(ctx, slog.NewRecord(houseTime, slog.LevelInfo, "m", 0))
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

// tsRE matches the house timestamp the slogtest run sees (record time, no clock).
var tsRE = regexp.MustCompile(`^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d\.\d{6} `)

// TestText_Slogtest runs the standard conformance suite, parsing each house line back into a map (groups
// from the dotted keys; a "G: " module prefix equal to the first group is dropped from the message).
func TestText_Slogtest(t *testing.T) {
	var buf bytes.Buffer
	newHandler := func(*testing.T) slog.Handler {
		buf.Reset()
		return logging.Text(&buf, logging.WithSource(slog.LevelError+4))
	}
	result := func(t *testing.T) map[string]any {
		line := strings.TrimSuffix(buf.String(), "\n")
		m := map[string]any{}
		lvl, rest, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("no level in %q", line)
		}
		m[slog.LevelKey] = lvl
		if loc := tsRE.FindStringIndex(rest); loc != nil {
			m[slog.TimeKey] = rest[:loc[1]-1]
			rest = rest[loc[1]:]
		}
		parts := strings.Split(rest, " | ")
		msg := parts[0]
		for _, p := range parts[1:] {
			k, v, _ := strings.Cut(p, "=")
			path := strings.Split(k, ".")
			cur := m
			for _, g := range path[:len(path)-1] {
				next, _ := cur[g].(map[string]any)
				if next == nil {
					next = map[string]any{}
					cur[g] = next
				}
				cur = next
			}
			cur[path[len(path)-1]] = v
		}
		if g, after, ok := strings.Cut(msg, ": "); ok {
			if _, isGroup := m[g].(map[string]any); isGroup {
				msg = after
			}
		}
		m[slog.MessageKey] = msg
		return m
	}
	slogtest.Run(t, newHandler, result)
}

// The handler is safe for concurrent use: whole lines, never interleaved, from derived handlers too.
func TestText_ConcurrentUnderRace(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var buf bytes.Buffer
	l := textLogger(&buf)
	const workers, each = 8, 200
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			ll := l.With("module", fmt.Sprintf("W%d", w)).WithGroup("g")
			for i := range each {
				if ctx.Err() != nil {
					return
				}
				ll.Info("tick", "i", i, "w", w)
			}
		})
	}
	wg.Wait()
	got := lines(buf.String())
	if len(got) != workers*each {
		t.Fatalf("lines = %d, want %d", len(got), workers*each)
	}
	re := regexp.MustCompile(`^INFO: ` + regexp.QuoteMeta(ts) + ` W(\d): tick \| g\.i=\d+ \| g\.w=(\d)$`)
	for _, ln := range got {
		m := re.FindStringSubmatch(ln)
		if m == nil || m[1] != m[2] {
			t.Fatalf("malformed line %q", ln)
		}
	}
}

func BenchmarkText(b *testing.B) {
	l := slog.New(logging.Text(discard{}, logging.WithClock(clock.NewFake(houseTime)))).With("module", "BENCH", "tenant_id", "t1")
	b.ReportAllocs()
	for b.Loop() {
		l.Info("request done", "status", 200, "path", "/v1/items")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// S1: an empty or nil module clears the module given earlier; a record's empty module clears a With one.
func TestText_S1_EmptyModuleClears(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf).With("module", "A")
	l.With("module", "").Info("x")
	l.With("module", nil).Info("y")
	l.Info("z", "module", "")
	l.Info("w")
	textLogger(&buf).WithGroup("g").With("module", "").Info("v")
	golden(t, buf.String(), "INFO: "+ts+" x\nINFO: "+ts+" y\nINFO: "+ts+" z\nINFO: "+ts+" A: w\nINFO: "+ts+" g: v | g.module=absent\n")
}
