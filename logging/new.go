package logging

import (
	"context"
	"io"
	"log/slog"
)

// New returns the house logger, ready for one-call setup (logging.New(os.Stdout, logging.ParseLevel(s))):
// Redact with the default secret words, then Context with no extractors, then the house Text handler, or
// slog.NewJSONHandler with WithJSON(true). level is applied first, so a WithLevel in opts overrides it. Both
// forms carry the same attributes, the same redaction, the same level and the same source rule (WithSource,
// default WARN and ERROR); the JSON form takes WithClock for its "time" and ignores WithColor and
// WithTimeFormat. New never calls slog.SetDefault; a nil w is io.Discard. For context attributes or extra
// keys and patterns, build the stack by hand: slog.New(Context(Redact(Text(w, opts...), ...), extract)).
//
// ID:          TB-LOG-021
// Pattern:     decorator stack (redaction over a slog output handler)
// Authority:   pkg.go.dev log/slog, "func New", "func NewJSONHandler", "type HandlerOptions"
// Authority:   Go team, "A Guide to Writing slog Handlers", golang/example slog-handler-guide
// Tags:        logging, slog, setup, house-format, redaction, json
func New(w io.Writer, level slog.Level, opts ...TextOption) *slog.Logger {
	cfg := newTextConfig(append([]TextOption{WithLevel(level)}, opts...))
	var out slog.Handler
	if cfg.json {
		out = newJSON(w, cfg)
	} else {
		out = newText(w, cfg)
	}
	return slog.New(Context(Redact(out), nil))
}

// newJSON is the JSON form: slog.NewJSONHandler with AddSource, gated to the source level, and the clock's
// time when WithClock is given.
func newJSON(w io.Writer, cfg textConfig) slog.Handler {
	if w == nil {
		w = io.Discard
	}
	clk := cfg.clk
	rep := func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) != 0 {
			return a
		}
		switch a.Key {
		case slog.TimeKey:
			if clk != nil && a.Value.Kind() == slog.KindTime {
				return slog.Time(slog.TimeKey, clk.Now())
			}
		case slog.SourceKey:
			if src, ok := a.Value.Any().(*slog.Source); ok && (src == nil || src.File == "") {
				return slog.Attr{} // a zero PC: dropped here too, whatever the Go version
			}
		}
		return a
	}
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: cfg.level, AddSource: true, ReplaceAttr: rep})
	return &sourceGate{next: h, min: cfg.srcMin}
}

// sourceGate drops the PC of records below min, so the JSON form shows source on the same levels as Text.
type sourceGate struct {
	next slog.Handler
	min  slog.Level
}

func (g *sourceGate) Enabled(ctx context.Context, l slog.Level) bool { return g.next.Enabled(ctx, l) }

func (g *sourceGate) Handle(ctx context.Context, r slog.Record) error {
	if r.Level < g.min {
		r.PC = 0
	}
	return g.next.Handle(ctx, r)
}

func (g *sourceGate) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &sourceGate{next: g.next.WithAttrs(attrs), min: g.min}
}

func (g *sourceGate) WithGroup(name string) slog.Handler {
	return &sourceGate{next: g.next.WithGroup(name), min: g.min}
}
