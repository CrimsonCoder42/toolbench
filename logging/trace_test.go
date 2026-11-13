package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/CrimsonCoder42/toolbench/logging"
)

// S4: the trace line shape; an empty value is absent.
func TestTrace_S4_Line(t *testing.T) {
	var buf bytes.Buffer
	logging.Trace(textLogger(&buf), "CART-CHECKOUT-PROBE", "B2", "cart loaded", "items", 3, "user_id", "")
	golden(t, buf.String(), "INFO: "+ts+" [CART-CHECKOUT-PROBE][B2] cart loaded | items=3 | user_id=absent\n")
}

// S4: an unpaired trailing key prints absent; nil prints absent; a non-string key is printed with %v.
func TestTrace_S4_UnpairedNilAndNonStringKey(t *testing.T) {
	var buf bytes.Buffer
	l := textLogger(&buf)
	logging.Trace(l, "T", "B1", "m", "a", nil, 7, "x", "tenant_id")
	logging.Trace(l, "T", "B2", "no kv")
	golden(t, buf.String(), "INFO: "+ts+" [T][B1] m | a=absent | 7=x | tenant_id=absent\nINFO: "+ts+" [T][B2] no kv\n")
}

// S4: tag and step are printed as given, never changed; a With module prefixes the line.
func TestTrace_S4_AsGivenAndModule(t *testing.T) {
	var buf bytes.Buffer
	logging.Trace(textLogger(&buf).With("module", "CART"), "xtenant-Probe", "", "m", "user_id", "u1")
	golden(t, buf.String(), "INFO: "+ts+" CART: [xtenant-Probe][] m | user_id=u1\n")
}

// S4: Trace logs at Info: nothing when the logger is above Info, nothing and no panic for a nil logger.
func TestTrace_S4_LevelAndNil(t *testing.T) {
	var buf bytes.Buffer
	logging.Trace(textLogger(&buf, logging.WithLevel(slog.LevelWarn)), "T", "B1", "m", "k", 1)
	logging.Trace(nil, "T", "B1", "m", "k", 1)
	golden(t, buf.String(), "")
	var rec logging.Recorder
	logging.Trace(slog.New(&rec), "T", "B1", "m")
	if rs := rec.Records(); len(rs) != 1 || rs[0].Level != slog.LevelInfo {
		t.Fatalf("records = %d, want one at Info", len(rs))
	}
}

// S2 + S4: with source lowered to Info, a Trace line names Trace's caller, not trace.go (text and JSON).
func TestTrace_S4_SourceIsTheCaller(t *testing.T) {
	var buf bytes.Buffer
	src := here()
	logging.Trace(textLogger(&buf, logging.WithSource(slog.LevelInfo)), "T", "B1", "m", "k", 1)
	golden(t, buf.String(), "INFO: "+ts+" "+src+"[T][B1] m | k=1\n")
	buf.Reset()
	logging.Trace(newLogger(&buf, logging.WithJSON(true), logging.WithSource(slog.LevelInfo)), "T", "B1", "m")
	if !strings.Contains(buf.String(), `trace_test.go","line":`) || strings.Contains(buf.String(), "trace.go") {
		t.Errorf("json source: %q", buf.String())
	}
}
