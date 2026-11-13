package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"sync"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// The house format's fixed parts (doc/specs/logging.md "House format").
const (
	// defaultTimeFormat is log.Ldate|log.Ltime|log.Lmicroseconds as a time layout.
	defaultTimeFormat = "2006/01/02 15:04:05.000000"
	// absent is printed for a missing, nil or empty value, so a missing field is visible.
	absent = "absent"
	// moduleKey is the top-level attribute printed as "MODULE: " before the message.
	moduleKey = "module"
	// errKey is the top-level attribute printed last.
	errKey = "err"
	// ANSI colours on the level prefix (S3).
	ansiBlue   = "\033[34m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiReset  = "\033[0m"
	// maxPooledBuf is the largest line buffer returned to the pool (the guide: keep only small buffers).
	maxPooledBuf = 16 << 10
)

// textConfig is what the TextOptions set. It is never changed after Text or New returns.
type textConfig struct {
	level   slog.Leveler
	color   bool
	srcMin  slog.Level
	timeFmt string
	clk     clock.Clock
	json    bool
}

// newTextConfig applies opts over the defaults: Info, no colour, source from Warn, the log package's time
// layout, the record's time, text output.
func newTextConfig(opts []TextOption) textConfig {
	c := textConfig{level: slog.LevelInfo, srcMin: slog.LevelWarn, timeFmt: defaultTimeFormat}
	for _, o := range opts {
		if o != nil {
			o(&c)
		}
	}
	return c
}

// TextOption configures Text and New.
//
// ID:          TB-LOG-012
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, logging, slog, house-format
type TextOption func(*textConfig)

// WithLevel sets the lowest level written; a *slog.LevelVar changes it while the program runs, with no race.
// Default slog.LevelInfo; a nil Leveler is ignored. In New it overrides the level argument.
//
// ID:          TB-LOG-013
// Pattern:     log level threshold (slog.Leveler)
// Authority:   pkg.go.dev log/slog, "type Leveler", "type LevelVar" and "type HandlerOptions" (field Level)
// Tags:        options, logging, slog, level
func WithLevel(l slog.Leveler) TextOption {
	return func(c *textConfig) {
		if l != nil {
			c.level = l
		}
	}
}

// WithColor colours the level prefix ("INFO: " blue, "WARN: " yellow, "ERROR: " red, "DEBUG: " none) with ANSI
// escape codes when on. Default off: no escape byte is ever written. Ignored by the JSON form.
//
// ID:          TB-LOG-014
// Pattern:     ANSI-coloured level prefix (terminal output)
// Authority:   pkg.go.dev log/slog, "type Level" (method String); ECMA-48, "Control Functions for Coded Character Sets", 5th ed., 1991, 8.3.117 SGR
// Tags:        options, logging, slog, color, terminal
func WithColor(on bool) TextOption {
	return func(c *textConfig) { c.color = on }
}

// WithSource prints the caller's file.go:line (and, in the JSON form, the source attribute) on records at min
// or above. Default slog.LevelWarn: WARN and ERROR lines carry it, DEBUG and INFO do not. A level above
// slog.LevelError (for example slog.LevelError+4) turns it off. A record whose PC is zero never carries it.
//
// ID:          TB-LOG-015
// Pattern:     source position on the record (slog.Source)
// Authority:   pkg.go.dev log/slog, "type HandlerOptions" (field AddSource), "type Source", "func (Record) Source"
// Tags:        options, logging, slog, source
func WithSource(min slog.Level) TextOption {
	return func(c *textConfig) { c.srcMin = min }
}

// WithTimeFormat sets the timestamp layout (package time layout). Default "2006/01/02 15:04:05.000000", the
// log package's Ldate|Ltime|Lmicroseconds; "" prints no timestamp. Ignored by the JSON form (RFC 3339).
//
// ID:          TB-LOG-016
// Pattern:     timestamp layout
// Authority:   pkg.go.dev time, "func (Time) AppendFormat"; pkg.go.dev log, "const Ldate"
// Tags:        options, logging, slog, time
func WithTimeFormat(layout string) TextOption {
	return func(c *textConfig) { c.timeFmt = layout }
}

// WithClock makes the timestamp of every record c.Now() instead of the record's own time, so tests with a
// clock.Fake get exact bytes. Default (or a nil c): the record's time, as slog set it.
//
// ID:          TB-LOG-017
// Pattern:     test double seam (injected time source)
// Authority:   pkg.go.dev log/slog, "type Record" (field Time)
// Tags:        options, logging, slog, clock, testing
func WithClock(c clock.Clock) TextOption {
	return func(cfg *textConfig) { cfg.clk = c }
}

// WithJSON makes New build the JSON form (slog.NewJSONHandler) instead of the text form: the production
// form, with the same attributes, redaction, level and source rule. Text ignores it.
//
// ID:          TB-LOG-018
// Pattern:     structured log output (JSON lines)
// Authority:   pkg.go.dev log/slog, "func NewJSONHandler"
// Tags:        options, logging, slog, json
func WithJSON(on bool) TextOption {
	return func(c *textConfig) { c.json = on }
}

// Text returns the house text handler, one line per record:
//
//	LEVEL: 2006/01/02 15:04:05.000000 file.go:42: MODULE: message | key=value | group.key=value | err=message
//
// LEVEL is the level's name (DEBUG, INFO, WARN, ERROR; slog's "INFO+2" between them), unpadded. The
// timestamp is the record's time (or WithClock's) in the WithTimeFormat layout; a zero record time and no
// clock prints none. file.go:42: is the caller, on WARN and ERROR only (WithSource). MODULE: is the value
// of the top-level "module" attribute as given (the record's wins over a With one, and an empty or nil one
// prints none), else the name of the logger's outermost group, else nothing: it is never invented. Every other attribute follows as
// " | key=value" in written order (With attributes first), groups flattened to group.key, values printed with
// fmt's %v and a nil, empty or "<nil>" value as "absent"; top-level "err" attributes go last. Empty
// attributes and groups are dropped (the Handler rules). Values are not quoted or escaped. WithColor colours
// the "LEVEL: " prefix. Attributes from WithAttrs are formatted once, when With is called. One mutex,
// shared by every handler derived from this one, guards each write, so lines never interleave; line
// buffers come from a pool. A nil w is io.Discard. Safe for concurrent use. Put Redact around it (New
// does).
//
// ID:          TB-LOG-011
// Pattern:     slog.Handler (text output, preformatted WithAttrs)
// Authority:   Go team, "A Guide to Writing slog Handlers", golang/example slog-handler-guide, "Implementing Handle", "Implementing WithGroup and WithAttrs", "Speed"
// Authority:   pkg.go.dev log/slog, "type Handler" (Handle rules)
// Errors:      w's Write error, unchanged
// Blocking:    while another record is being written, and as long as w.Write
// Tags:        logging, slog, handler, text, house-format
func Text(w io.Writer, opts ...TextOption) slog.Handler {
	return newText(w, newTextConfig(opts))
}

// newText builds the text handler from a finished config.
func newText(w io.Writer, cfg textConfig) *textHandler {
	if w == nil {
		w = io.Discard
	}
	return &textHandler{out: &textOut{w: w, cfg: cfg}}
}

// textOut is shared by a text handler and every handler derived from it.
type textOut struct {
	w    io.Writer
	cfg  textConfig
	mu   sync.Mutex // guards writes to w
	bufs sync.Pool  // *[]byte line buffers
}

// textHandler is the handler Text returns. Every field is immutable after construction.
type textHandler struct {
	out         *textOut
	pre         []byte // " | key=value" from WithAttrs, formatted once
	errs        []byte // top-level err attributes from WithAttrs, printed last
	module      string // the module from WithAttrs ("" when set empty)
	moduleSet   bool   // a top-level module attribute was given to WithAttrs, even an empty one
	groupModule string // the outermost group's name
	prefix      string // "g1.g2." for the open groups
}

func (h *textHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.out.cfg.level.Level()
}

func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	cfg := &h.out.cfg
	bp, _ := h.out.bufs.Get().(*[]byte)
	if bp == nil {
		b := make([]byte, 0, 512)
		bp = &b
	}
	buf := (*bp)[:0]

	buf = appendLevel(buf, r.Level, cfg.color)
	if cfg.timeFmt != "" {
		t := r.Time
		if cfg.clk != nil {
			t = cfg.clk.Now()
		}
		if !t.IsZero() {
			buf = t.AppendFormat(buf, cfg.timeFmt)
			buf = append(buf, ' ')
		}
	}
	if r.Level >= cfg.srcMin && r.PC != 0 {
		buf = appendSource(buf, r.PC)
	}
	top := h.prefix == "" // record attributes are at the top level
	// The nearest module given wins, even an empty one (which prints none): record, then With, then group.
	module, found := "", false
	if top {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == moduleKey && a.Value.Resolve().Kind() != slog.KindGroup {
				module, _ = moduleText(a.Value)
				found = true
			}
			return true
		})
	}
	switch {
	case found:
	case h.moduleSet:
		module = h.module
	default:
		module = h.groupModule
	}
	if module != "" {
		buf = append(buf, module...)
		buf = append(buf, ": "...)
	}
	buf = append(buf, r.Message...)
	buf = append(buf, h.pre...)
	r.Attrs(func(a slog.Attr) bool {
		if top && (a.Key == errKey || a.Key == moduleKey && a.Value.Resolve().Kind() != slog.KindGroup) {
			return true // the module is printed before the message, errs last
		}
		buf = appendAttr(buf, h.prefix, a)
		return true
	})
	buf = append(buf, h.errs...)
	if top {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == errKey {
				buf = appendAttr(buf, "", a)
			}
			return true
		})
	}
	buf = append(buf, '\n')

	h.out.mu.Lock()
	_, err := h.out.w.Write(buf)
	h.out.mu.Unlock()

	if cap(buf) <= maxPooledBuf {
		*bp = buf
		h.out.bufs.Put(bp)
	}
	return err
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	nh := *h
	nh.pre = append([]byte(nil), h.pre...)
	nh.errs = append([]byte(nil), h.errs...)
	for _, a := range attrs {
		if h.prefix == "" {
			switch a.Key {
			case moduleKey:
				if a.Value.Resolve().Kind() == slog.KindGroup {
					break // a group named module is an ordinary attribute
				}
				nh.module, _ = moduleText(a.Value) // an empty or nil module clears it
				nh.moduleSet = true
				continue
			case errKey:
				nh.errs = appendAttr(nh.errs, "", a)
				continue
			}
		}
		nh.pre = appendAttr(nh.pre, h.prefix, a)
	}
	return &nh
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	if h.prefix == "" {
		nh.groupModule = name
	}
	nh.prefix = h.prefix + name + "."
	return &nh
}

// moduleText returns the printed module for v, or false when v is a group or prints as absent.
func moduleText(v slog.Value) (string, bool) {
	v = v.Resolve()
	if v.Kind() == slog.KindGroup {
		return "", false
	}
	s := string(appendValue(nil, v))
	if s == absent {
		return "", false
	}
	return s, true
}

// appendLevel writes "LEVEL: ", coloured when color is on.
func appendLevel(buf []byte, l slog.Level, color bool) []byte {
	code := ""
	if color {
		switch {
		case l >= slog.LevelError:
			code = ansiRed
		case l >= slog.LevelWarn:
			code = ansiYellow
		case l >= slog.LevelInfo:
			code = ansiBlue
		}
	}
	buf = append(buf, code...)
	buf = append(buf, l.String()...)
	buf = append(buf, ": "...)
	if code != "" {
		buf = append(buf, ansiReset...)
	}
	return buf
}

// appendSource writes "file.go:line: " for pc, the log package's Lshortfile form; nothing when unknown.
func appendSource(buf []byte, pc uintptr) []byte {
	f, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	if f.File == "" {
		return buf
	}
	file := f.File
	for i := len(file) - 1; i > 0; i-- {
		if file[i] == '/' {
			file = file[i+1:]
			break
		}
	}
	buf = append(buf, file...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, int64(f.Line), 10)
	return append(buf, ": "...)
}

// appendAttr writes " | prefix+key=value" for a, resolved, with groups flattened and empty ones dropped.
func appendAttr(buf []byte, prefix string, a slog.Attr) []byte {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return buf
	}
	if a.Value.Kind() == slog.KindGroup {
		g := a.Value.Group()
		if len(g) == 0 {
			return buf
		}
		p := prefix
		if a.Key != "" {
			p = prefix + a.Key + "."
		}
		for _, ga := range g {
			buf = appendAttr(buf, p, ga)
		}
		return buf
	}
	buf = append(buf, " | "...)
	buf = append(buf, prefix...)
	buf = append(buf, a.Key...)
	buf = append(buf, '=')
	return appendValue(buf, a.Value)
}

// appendValue writes v as fmt's %v would, or "absent" when that is empty or "<nil>" (a nil value).
func appendValue(buf []byte, v slog.Value) []byte {
	start := len(buf)
	switch v.Kind() {
	case slog.KindString:
		buf = append(buf, v.String()...)
	case slog.KindInt64:
		buf = strconv.AppendInt(buf, v.Int64(), 10)
	case slog.KindUint64:
		buf = strconv.AppendUint(buf, v.Uint64(), 10)
	case slog.KindFloat64:
		buf = strconv.AppendFloat(buf, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		buf = strconv.AppendBool(buf, v.Bool())
	case slog.KindDuration:
		buf = append(buf, v.Duration().String()...)
	case slog.KindTime:
		buf = append(buf, v.Time().String()...)
	default:
		if x := v.Any(); x != nil {
			buf = fmt.Append(buf, x)
		}
	}
	if s := buf[start:]; len(s) == 0 || string(s) == "<nil>" {
		buf = append(buf[:start], absent...)
	}
	return buf
}
