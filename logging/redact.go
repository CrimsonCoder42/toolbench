package logging

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// defaultMask replaces a masked value or a pattern match when WithMask is not given.
const defaultMask = "[REDACTED]"

// defaultWords returns the words that make a key secret when its normalised form contains one of them (the
// same list as config's default secret words). A function, not a package variable, so the
// package has no mutable global state.
func defaultWords() []string {
	return []string{
		"password", "passwd", "passphrase", "secret", "token", "key", "credential", "private", "dsn",
		"authorization", "cookie",
	}
}

// normKey lower-cases k and drops '-', '_', '.' and ' ', so "API-Key", "api_key" and "apiKey" compare equal.
func normKey(k string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '-', '_', '.', ' ':
			return -1
		}
		return r
	}, strings.ToLower(k))
}

// redactConfig is what the options set. It is never changed after Redact returns.
type redactConfig struct {
	words    []string        // default secret words: a normalised key containing one is secret
	keys     map[string]bool // WithKeys: exact normalised keys
	patterns []*regexp.Regexp
	mask     string
}

// RedactOption configures Redact.
//
// ID:          TB-LOG-002
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, logging, redaction
type RedactOption func(*redactConfig)

// WithKeys adds attribute keys whose values Redact masks. Keys are compared after normalising (lower case,
// '-', '_', '.' and ' ' removed) and must match exactly: WithKeys("ssn") masks "SSN" and "s_s_n", not
// "ssn_hint". The default secret words stay on: a key whose normalised form contains password, passwd,
// passphrase, secret, token, key, credential, private, dsn, authorization or cookie is always masked ("API_KEY", "access-token",
// "db.dsn"; over-matching such as "token_count" is accepted, as in config). Empty keys are ignored.
//
// ID:          TB-LOG-003
// Pattern:     log redaction (key masking)
// Authority:   pkg.go.dev log/slog, "type HandlerOptions" (field ReplaceAttr) and "type LogValuer"
// Tags:        options, logging, redaction, secrets
func WithKeys(keys ...string) RedactOption {
	return func(c *redactConfig) {
		for _, k := range keys {
			if n := normKey(k); n != "" {
				c.keys[n] = true
			}
		}
	}
}

// WithPatterns masks every match of each regexp in any text a record carries: the message, keys, group names,
// string values, and the text forms of other values (fmt %v and %+v, MarshalText, json.Marshal, a []byte as
// text), which covers a secret inside an error message, a fmt.Stringer, a struct or a LogValuer result. A
// non-string value whose text matches is replaced by the first matching text form (in the order %+v, %v,
// []byte, MarshalText, JSON) with the matches masked. Nil regexps are
// skipped. Default: no patterns.
//
// ID:          TB-LOG-004
// Pattern:     log redaction (value pattern masking)
// Authority:   pkg.go.dev regexp, "func (*Regexp) ReplaceAllLiteralString"; pkg.go.dev log/slog, "func (Value) Resolve"
// Tags:        options, logging, redaction, secrets, regexp
func WithPatterns(res ...*regexp.Regexp) RedactOption {
	return func(c *redactConfig) {
		for _, re := range res {
			if re != nil {
				c.patterns = append(c.patterns, re)
			}
		}
	}
}

// WithMask sets the text that replaces a masked value or a pattern match. Default "[REDACTED]"; an empty mask is
// allowed and removes the text.
//
// ID:          TB-LOG-005
// Pattern:     log redaction (mask text)
// Authority:   pkg.go.dev log/slog, "type HandlerOptions" (field ReplaceAttr)
// Tags:        options, logging, redaction
func WithMask(mask string) RedactOption {
	return func(c *redactConfig) { c.mask = mask }
}

// Redact wraps h so that no secret reaches it: the value of an attribute whose key is a secret key (WithKeys,
// or containing a default secret word) is replaced by the mask, at any group depth, in records and in WithAttrs; every attribute
// logged under a WithGroup whose name is a secret key is masked; and every match of a WithPatterns regexp is
// masked in the message and in every value's text. Values are resolved (LogValuer) before the rules, so a
// LogValuer cannot carry a secret past them. Enabled is h's; WithAttrs redacts and then calls h.WithAttrs, so
// pre-formatting in h still works; WithGroup("") returns the receiver. The caller's Record is never changed.
// A nil h is slog.DiscardHandler. The handler is immutable and safe for concurrent use.
//
// ID:          TB-LOG-001
// Pattern:     decorator (wrapping slog.Handler) for log redaction
// Authority:   Go team, "A Guide to Writing slog Handlers", golang/example slog-handler-guide, "Implementing WithGroup and WithAttrs"
// Authority:   pkg.go.dev log/slog, "type Handler" (Handle rules), "type LogValuer", "func (Value) Resolve"
// Errors:      the wrapped handler's Handle error, unchanged
// Blocking:    only as much as h's Handle
// Tags:        logging, slog, redaction, secrets, handler, decorator
func Redact(h slog.Handler, opts ...RedactOption) slog.Handler {
	if h == nil {
		h = slog.DiscardHandler
	}
	c := &redactConfig{words: defaultWords(), keys: map[string]bool{}, mask: defaultMask}
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	return &redactHandler{next: h, cfg: c}
}

// redactHandler is the handler Redact returns. maskAll is set under a WithGroup whose name is a secret key.
type redactHandler struct {
	next    slog.Handler
	cfg     *redactConfig
	maskAll bool
}

func (h *redactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	nr := slog.NewRecord(r.Time, r.Level, h.cfg.text(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		nr.AddAttrs(h.attr(a))
		return true
	})
	return h.next.Handle(ctx, nr)
}

func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, h.attr(a))
	}
	return &redactHandler{next: h.next.WithAttrs(out), cfg: h.cfg, maskAll: h.maskAll}
}

func (h *redactHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &redactHandler{
		next:    h.next.WithGroup(h.cfg.text(name)),
		cfg:     h.cfg,
		maskAll: h.maskAll || h.cfg.secret(name),
	}
}

// attr returns a with its value resolved and every rule applied, recursively into groups.
func (h *redactHandler) attr(a slog.Attr) slog.Attr {
	if a.Equal(slog.Attr{}) {
		return a
	}
	a.Value = a.Value.Resolve()
	key := h.cfg.text(a.Key)
	if h.maskAll || h.cfg.secret(a.Key) {
		return slog.String(key, h.cfg.mask)
	}
	a.Key = key
	switch a.Value.Kind() {
	case slog.KindGroup:
		in := a.Value.Group()
		out := make([]slog.Attr, 0, len(in))
		for _, g := range in {
			out = append(out, h.attr(g))
		}
		a.Value = slog.GroupValue(out...)
	case slog.KindString:
		a.Value = slog.StringValue(h.cfg.text(a.Value.String()))
	case slog.KindAny:
		a.Value = h.cfg.anyValue(a.Value)
	}
	return a
}

// secret reports whether key is a secret key: an exact WithKeys key or one containing a default word.
func (c *redactConfig) secret(key string) bool {
	n := normKey(key)
	if c.keys[n] {
		return true
	}
	for _, w := range c.words {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// text masks every pattern match in s.
func (c *redactConfig) text(s string) string {
	for _, re := range c.patterns {
		s = re.ReplaceAllLiteralString(s, c.mask)
	}
	return s
}

// matches reports whether any pattern matches s.
func (c *redactConfig) matches(s string) bool {
	for _, re := range c.patterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// anyValue returns v unchanged unless one of its text forms matches a pattern; then it returns the first
// matching form with the matches masked, so no output handler (JSON or text) can print the secret.
func (c *redactConfig) anyValue(v slog.Value) slog.Value {
	x := v.Any()
	if x == nil || len(c.patterns) == 0 {
		return v
	}
	for _, s := range renderings(x) {
		if c.matches(s) {
			return slog.StringValue(c.text(s))
		}
	}
	return v
}

// renderings returns the text forms of x that the standard output handlers and fmt may print: %+v, %v, a
// []byte as text, MarshalText (TextHandler) and json.Marshal (JSONHandler, which follows pointers %+v does
// not). A panic in caller marshalling code is recovered through panics and that form is skipped.
func renderings(x any) []string {
	out := []string{fmt.Sprintf("%+v", x), fmt.Sprintf("%v", x)}
	if b, ok := x.([]byte); ok {
		out = append(out, string(b))
	}
	if tm, ok := x.(encoding.TextMarshaler); ok {
		if b, err := panics.TryValue(tm.MarshalText); err == nil {
			out = append(out, string(b))
		}
	}
	if b, err := panics.TryValue(func() ([]byte, error) { return json.Marshal(x) }); err == nil {
		out = append(out, string(b))
	}
	return out
}
