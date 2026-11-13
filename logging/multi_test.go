package logging_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"testing/slogtest"

	"github.com/CrimsonCoder42/toolbench/logging"
)

func TestMulti_Slogtest(t *testing.T) {
	var buf *bytes.Buffer
	slogtest.Run(t, func(*testing.T) slog.Handler {
		buf = new(bytes.Buffer)
		return logging.Multi(nil, slog.NewJSONHandler(buf, nil), slog.DiscardHandler)
	}, func(t *testing.T) map[string]any { return decodeOne(t, buf) })
}

func TestMulti_FanOutPerHandlerLevel(t *testing.T) {
	var debug, warn bytes.Buffer
	var rec logging.Recorder
	h := logging.Multi(
		slog.NewJSONHandler(&debug, &slog.HandlerOptions{Level: slog.LevelDebug, ReplaceAttr: noTime}),
		nil,
		slog.NewJSONHandler(&warn, &slog.HandlerOptions{Level: slog.LevelWarn, ReplaceAttr: noTime}),
		&rec,
	)
	l := slog.New(h).With("svc", "a").WithGroup("g")
	l.Debug("d", "n", 1)
	l.Warn("w", "n", 2)
	if got := len(lines(debug.String())); got != 2 {
		t.Fatalf("debug handler lines = %d: %s", got, withheld(debug.String()))
	}
	if got := lines(warn.String()); len(got) != 1 || got[0] != `{"level":"WARN","msg":"w","svc":"a","g":{"n":2}}` {
		t.Fatalf("warn handler got %d lines: %s", len(got), withheld(warn.String()))
	}
	if got := len(rec.Records()); got != 2 {
		t.Fatalf("recorder records = %d", got)
	}
}

func TestMulti_Enabled(t *testing.T) {
	ctx := context.Background()
	warnOnly := slog.NewJSONHandler(new(bytes.Buffer), &slog.HandlerOptions{Level: slog.LevelWarn})
	if logging.Multi().Enabled(ctx, slog.LevelError) {
		t.Fatal("no handlers: never enabled")
	}
	if logging.Multi(nil, nil).Enabled(ctx, slog.LevelError) {
		t.Fatal("only nil handlers: never enabled")
	}
	h := logging.Multi(slog.DiscardHandler, warnOnly)
	if h.Enabled(ctx, slog.LevelInfo) || !h.Enabled(ctx, slog.LevelWarn) {
		t.Fatal("Enabled must be true when any handler is enabled, false otherwise")
	}
}

// failing is a handler whose Handle fails with err.
type failing struct {
	slog.Handler
	err error
}

func (f failing) Handle(context.Context, slog.Record) error { return f.err }

func TestMulti_ErrorsJoined(t *testing.T) {
	e1, e2 := errors.New("one"), errors.New("two")
	var rec logging.Recorder
	h := logging.Multi(failing{&rec, e1}, &rec, failing{&rec, e2})
	err := h.Handle(context.Background(), slog.NewRecord(fixed, slog.LevelInfo, "m", 0))
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Fatalf("err = %v", err)
	}
	if got := len(rec.Records()); got != 1 {
		t.Fatalf("the healthy handler still gets the record: %d", got)
	}
}
