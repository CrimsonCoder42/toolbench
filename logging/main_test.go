package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/logging"
)

func TestMain(m *testing.M) { os.Exit(leak.Main(m)) }

// fixed is the time every hand-made record carries (no real clock in these tests).
var fixed = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// testCtx bounds a test (go-testing.md: at most 10 s).
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// noTime drops the top-level time attribute so output is deterministic.
func noTime(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && len(groups) == 0 {
		return slog.Attr{}
	}
	return a
}

// jsonTo is a JSON handler at Debug on buf with the time removed.
func jsonTo(buf *bytes.Buffer) slog.Handler {
	return slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug, ReplaceAttr: noTime})
}

// textTo is a text handler at Debug on buf with the time removed.
func textTo(buf *bytes.Buffer) slog.Handler {
	return slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug, ReplaceAttr: noTime})
}

// render prints every record of r through both stdlib output handlers and returns the full text, so a
// search covers the message, every key, every group and every value as a real handler prints it.
func render(t *testing.T, r *logging.Recorder) string {
	t.Helper()
	var buf bytes.Buffer
	for _, rec := range r.Records() {
		if err := jsonTo(&buf).Handle(context.Background(), rec); err != nil {
			t.Fatalf("json render failed: %T", err)
		}
		if err := textTo(&buf).Handle(context.Background(), rec); err != nil {
			t.Fatalf("text render failed: %T", err)
		}
	}
	return buf.String()
}

// decodeOne decodes the single JSON line in buf.
func decodeOne(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		return map[string]any{}
	}
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("want one line, got %s", withheld(line))
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("decode %s: %T", withheld(line), err)
	}
	return m
}

// lines splits handler output into its non-empty lines.
func lines(s string) []string {
	var out []string
	for l := range strings.Lines(s) {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// withheld stands in for test output in a failure message: only its length is printed, never its text, so a
// failing assertion cannot print a secret (wave 6 duty 2).
func withheld(s string) string {
	return fmt.Sprintf("<%d bytes withheld>", len(s))
}
