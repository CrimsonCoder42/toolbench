package logging_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/logging"
)

// houseSecret is given to New through every secret-keyed path; it must never appear in any output.
const houseSecret = "pw_Hz7Kq2Vx9Lm"

// newLogger is New on buf at Debug with the fixed clock, plus opts.
func newLogger(buf *bytes.Buffer, opts ...logging.TextOption) *slog.Logger {
	return logging.New(buf, slog.LevelDebug, append([]logging.TextOption{logging.WithClock(clock.NewFake(houseTime))}, opts...)...)
}

// The request's three sample lines, rendered through New with the fixed clock, byte for byte.
func TestNew_RequestSamples(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf)
	l.Info("ListAuthUsers SUCCESS", "module", "AUTH_USER", "tenant_id", "e1c6acea", "active", 0, "pending", 0)
	logging.Trace(l, "CART-CHECKOUT-PROBE", "B2", "cart loaded", "items", 3, "user_id", "")
	src := here()
	l.Error("drug names unavailable", "module", "FEED", "err", errors.New("upstream timeout"))
	golden(t, buf.String(),
		"INFO: "+ts+" AUTH_USER: ListAuthUsers SUCCESS | tenant_id=e1c6acea | active=0 | pending=0\n"+
			"INFO: "+ts+" [CART-CHECKOUT-PROBE][B2] cart loaded | items=3 | user_id=absent\n"+
			"ERROR: "+ts+" "+src+"FEED: drug names unavailable | err=upstream timeout\n")
}

// secretCalls logs houseSecret under secret keys through every path New's stack takes.
func secretCalls(l *slog.Logger) {
	l.Info("login", "module", "AUTH", "password", houseSecret, "user_id", "u1")
	l.With("api_key", houseSecret).Warn("with", "n", 1)
	l.Error("db", slog.Group("db", "Password", houseSecret, "host", "h"), "err", errors.New("failed"))
	l.WithGroup("credentials").Info("group", "value", houseSecret)
	logging.Trace(l, "T", "B1", "trace", "access_token", houseSecret, "authorization", houseSecret)
}

// S6: a secret given to New never appears in the text or the JSON output (failures print lengths only).
func TestNew_S6_SecretNeverInTextOrJSON(t *testing.T) {
	for _, json := range []bool{false, true} {
		var buf bytes.Buffer
		secretCalls(newLogger(&buf, logging.WithJSON(json)))
		out := buf.String()
		if strings.Contains(out, houseSecret) || strings.Contains(out, houseSecret[3:]) {
			t.Errorf("json=%v: the secret is in the output %s", json, withheld(out))
		}
		if got := len(lines(out)); got != 5 {
			t.Errorf("json=%v: %d lines, want 5", json, got)
		}
		if got := strings.Count(out, "[REDACTED]"); got != 6 {
			t.Errorf("json=%v: %d masks, want 6", json, got)
		}
	}
}

// textPairs returns the module and the " | key=value" pairs of one house text line, keyed by dotted path.
func textPairs(line string) (string, map[string]string) {
	parts := strings.Split(line, " | ")
	m := map[string]string{}
	for _, p := range parts[1:] {
		k, v, _ := strings.Cut(p, "=")
		m[k] = v
	}
	module := ""
	if i := strings.Index(parts[0], ": "); i >= 0 { // after "LEVEL: "
		head := parts[0][i+2:]
		if f := strings.Fields(head); len(f) > 2 && strings.HasSuffix(f[2], ":") {
			module = strings.TrimSuffix(f[2], ":") // "date time MODULE: msg"
		}
	}
	return module, m
}

// jsonPairs flattens one JSON line to dotted keys, dropping the built-ins; null and "" become "absent".
func jsonPairs(t *testing.T, line string) (string, map[string]string) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("decode %s: %T", withheld(line), err)
	}
	module, _ := m["module"].(string)
	for _, k := range []string{"time", "level", "msg", "source", "module"} {
		delete(m, k)
	}
	out := map[string]string{}
	var walk func(prefix string, v map[string]any)
	walk = func(prefix string, v map[string]any) {
		for k, x := range v {
			switch x := x.(type) {
			case map[string]any:
				walk(prefix+k+".", x)
			case nil:
				out[prefix+k] = "absent"
			default:
				s := fmt.Sprint(x)
				if s == "" {
					s = "absent"
				}
				out[prefix+k] = s
			}
		}
	}
	walk("", m)
	return module, out
}

// S7: the same calls give the same attributes, the same module and the same masks in text and JSON.
func TestNew_S7_TextAndJSONSameAttrs(t *testing.T) {
	calls := func(l *slog.Logger) {
		secretCalls(l)
		l.Info("plain", "module", "FEED", "count", 3, "ok", true, "empty", "", "none", nil)
		l.With("tenant_id", "t1").WithGroup("req").Info("nested", "path", "/v1", slog.Group("q", "page", 2))
	}
	var tb, jb bytes.Buffer
	calls(newLogger(&tb, logging.WithSource(slog.LevelError+4)))
	calls(newLogger(&jb, logging.WithJSON(true), logging.WithSource(slog.LevelError+4)))
	tl, jl := lines(tb.String()), lines(jb.String())
	if len(tl) != len(jl) || len(tl) != 7 {
		t.Fatalf("text %d lines, json %d lines, want 7 each", len(tl), len(jl))
	}
	for i := range tl {
		tm, tp := textPairs(tl[i])
		jm, jp := jsonPairs(t, jl[i])
		if tm != jm && !(jm == "" && strings.HasPrefix(tm, "credentials") || jm == "" && tm == "req") {
			t.Errorf("line %d: module text %d bytes, json %d bytes", i, len(tm), len(jm))
		}
		if !maps.Equal(tp, jp) {
			t.Errorf("line %d: text keys %v, json keys %v (values withheld)", i, slices.Sorted(maps.Keys(tp)), slices.Sorted(maps.Keys(jp)))
		}
	}
}

// S7: the JSON form takes the clock's time, and shows source on WARN and ERROR only.
func TestNew_S7_JSONTimeAndSource(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, logging.WithJSON(true))
	l.Info("i")
	l.Error("e")
	ls := lines(buf.String())
	if len(ls) != 2 {
		t.Fatalf("%d lines, want 2", len(ls))
	}
	var info, errl map[string]any
	if json.Unmarshal([]byte(ls[0]), &info) != nil || json.Unmarshal([]byte(ls[1]), &errl) != nil {
		t.Fatal("undecodable line")
	}
	if info["time"] != "2026-11-13T09:04:05.678901Z" {
		t.Errorf("time = %v", info["time"])
	}
	if _, ok := info["source"]; ok {
		t.Error("source on an INFO line")
	}
	src, _ := errl["source"].(map[string]any)
	if f, _ := src["file"].(string); !strings.HasSuffix(f, "new_test.go") {
		t.Errorf("ERROR source = %v, want new_test.go", errl["source"])
	}
}

// New: the level argument, a WithLevel override, the default (no colour, text) and a nil writer.
func TestNew_LevelOverrideAndNilWriter(t *testing.T) {
	var buf bytes.Buffer
	l := logging.New(&buf, slog.LevelWarn, logging.WithClock(clock.NewFake(houseTime)))
	l.Info("hidden")
	l.Warn("w")
	logging.New(&buf, slog.LevelError, logging.WithClock(clock.NewFake(houseTime)), logging.WithLevel(slog.LevelDebug),
		logging.WithSource(slog.LevelError+4)).Debug("d")
	ls := lines(buf.String())
	if len(ls) != 2 || !strings.HasPrefix(ls[0], "WARN: "+ts+" new_test.go:") || ls[1] != "DEBUG: "+ts+" d" {
		t.Errorf("got %q", ls)
	}
	logging.New(nil, slog.LevelInfo).Info("to discard")
	logging.New(nil, slog.LevelInfo, logging.WithJSON(true)).Info("to discard")
}
