package logging_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"testing/slogtest"

	"github.com/CrimsonCoder42/toolbench/ctxkey"
	"github.com/CrimsonCoder42/toolbench/logging"
	"github.com/CrimsonCoder42/toolbench/panics"
)

func TestContext_Slogtest(t *testing.T) {
	extracts := map[string]func(context.Context) []slog.Attr{
		"none":   func(context.Context) []slog.Attr { return nil },
		"always": func(context.Context) []slog.Attr { return []slog.Attr{slog.String("rid", "r-1")} },
	}
	for name, ex := range extracts {
		t.Run(name, func(t *testing.T) {
			var buf *bytes.Buffer
			slogtest.Run(t, func(*testing.T) slog.Handler {
				buf = new(bytes.Buffer)
				return logging.Context(slog.NewJSONHandler(buf, nil), ex)
			}, func(t *testing.T) map[string]any { return decodeOne(t, buf) })
		})
	}
}

// TestContext_CtxkeyValuesInEveryRecord: every record, at every WithGroup/With depth, carries the ctxkey values
// at the root, before the logger's own attrs.
func TestContext_CtxkeyValuesInEveryRecord(t *testing.T) {
	ctx := testCtx(t)
	rid := ctxkey.New[string]("rid")
	user := ctxkey.New[int]("user")
	var buf bytes.Buffer
	h := logging.Context(jsonTo(&buf), logging.Combine(logging.FromKey("request_id", rid), nil, logging.FromKey("user_id", user)))
	ctx = user.With(rid.With(ctx, "r-9"), 42)
	base := slog.New(h)
	loggers := []*slog.Logger{
		base,
		base.With("a", 1),
		base.WithGroup("g"),
		base.WithGroup("g").With("b", 2).WithGroup("h"),
	}
	for _, l := range loggers {
		l.InfoContext(ctx, "m", "k", "v")
		l.LogAttrs(ctx, slog.LevelWarn, "m2")
	}
	ls := lines(buf.String())
	if len(ls) != 2*len(loggers) {
		t.Fatalf("lines = %d: %s", len(ls), withheld(buf.String()))
	}
	for _, line := range ls {
		if !strings.Contains(line, `"msg":"m`) || !strings.Contains(line, `,"request_id":"r-9","user_id":42`) {
			t.Fatalf("context attrs missing or not at root first: %s", withheld(line))
		}
	}
	if want := `{"level":"INFO","msg":"m","request_id":"r-9","user_id":42,"g":{"b":2,"h":{"k":"v"}}}`; ls[6] != want {
		t.Fatalf("line 6 differs from %s: %s", want, withheld(ls[6]))
	}
}

func TestContext_NoValueNoAttr(t *testing.T) {
	rid := ctxkey.New[string]("rid")
	var buf bytes.Buffer
	l := slog.New(logging.Context(jsonTo(&buf), logging.FromKey("request_id", rid)))
	l.InfoContext(testCtx(t), "m")
	if got := strings.TrimSpace(buf.String()); got != `{"level":"INFO","msg":"m"}` {
		t.Fatalf("unexpected output: %s", withheld(got))
	}
}

func TestContext_NilInputs(t *testing.T) {
	var rec logging.Recorder
	if h := logging.Context(&rec, nil); h != slog.Handler(&rec) {
		t.Fatal("nil extract should return h")
	}
	h := logging.Context(nil, func(context.Context) []slog.Attr { return nil })
	if h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("nil h should be a discard handler")
	}
	var nilKey *ctxkey.Key[string]
	if got := logging.FromKey("x", nilKey)(context.Background()); got != nil {
		t.Fatalf("nil key gave %d attrs", len(got))
	}
	// A nil ctx passed straight to Handle is context.Background().
	seen := false
	h2 := logging.Context(&rec, func(ctx context.Context) []slog.Attr { seen = ctx != nil; return nil })
	if err := h2.Handle(nil, slog.NewRecord(fixed, slog.LevelInfo, "m", 0)); err != nil || !seen {
		t.Fatalf("err=%v seen=%v", err, seen)
	}
}

func TestContext_PanickingExtract(t *testing.T) {
	var rec logging.Recorder
	h := logging.Context(&rec, func(context.Context) []slog.Attr { panic("extract broke") })
	err := h.Handle(testCtx(t), slog.NewRecord(fixed, slog.LevelInfo, "still logged", 0))
	if !errors.Is(err, panics.ErrPanic) {
		t.Fatalf("err = %v, want ErrPanic", err)
	}
	rs := rec.Records()
	if len(rs) != 1 || rs[0].Message != "still logged" {
		t.Fatalf("record not passed on: %d records", len(rs))
	}
}

func TestContext_EnabledDelegates(t *testing.T) {
	var lv slog.LevelVar
	lv.Set(slog.LevelError)
	var buf bytes.Buffer
	h := logging.Context(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: &lv}),
		func(context.Context) []slog.Attr { return nil })
	ctx := context.Background()
	if h.Enabled(ctx, slog.LevelWarn) || !h.WithGroup("g").Enabled(ctx, slog.LevelError) {
		t.Fatal("Enabled not delegated")
	}
}

// TestContext_ExtractedValuesAreRedacted: with the documented order, Context(Redact(h)), values taken from the
// context pass through Redact, by key and by pattern, under a group too.
func TestContext_ExtractedValuesAreRedacted(t *testing.T) {
	tok := ctxkey.New[string]("tok")
	re := regexp.MustCompile(`tok_\w+`)
	ctx := tok.With(testCtx(t), "tok_CTX1")
	ex := logging.Combine(logging.FromKey("auth", tok), logging.FromKey("Token", tok))
	var a bytes.Buffer
	l := slog.New(logging.Context(logging.Redact(jsonTo(&a), logging.WithPatterns(re)), ex))
	l.InfoContext(ctx, "m")
	l.WithGroup("g").With("x", 1).InfoContext(ctx, "m")
	if strings.Contains(a.String(), "tok_CTX1") || strings.Count(a.String(), "[REDACTED]") != 4 {
		t.Fatalf("Context(Redact) leaked or under-masked: %s", withheld(a.String()))
	}
}

func TestContext_DoesNotModifyReceiver(t *testing.T) {
	var buf bytes.Buffer
	base := logging.Context(jsonTo(&buf), func(context.Context) []slog.Attr { return []slog.Attr{slog.Int("c", 1)} })
	_ = base.WithGroup("g").WithAttrs([]slog.Attr{slog.Int("x", 1)})
	if same := base.WithGroup(""); same != base {
		t.Fatal(`WithGroup("") should return the receiver`)
	}
	slog.New(base).Info("m")
	if got := strings.TrimSpace(buf.String()); got != `{"level":"INFO","msg":"m","c":1}` {
		t.Fatalf("unexpected output: %s", withheld(got))
	}
}

func BenchmarkContext(b *testing.B) {
	rid := ctxkey.New[string]("rid")
	var rec logging.Recorder
	h := logging.Context(&rec, logging.FromKey("request_id", rid)).WithGroup("g")
	ctx := rid.With(context.Background(), "r")
	r := slog.NewRecord(fixed, slog.LevelInfo, "m", 0)
	b.ReportAllocs()
	for b.Loop() {
		_ = h.Handle(ctx, r)
		rec.Reset()
	}
}
