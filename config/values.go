package config

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// redacted replaces the value of a secret key in every printed form of Values.
const redacted = "[REDACTED]"

// defaultSecretWords returns the words that mark a key as secret when any of them occurs in its normalised
// form (lower case, '-', '_', '.' and ' ' dropped, as the logging package normalises keys): "db.password", "API_KEY", "auth.token", "stripe.secret_key", "db.dsn" (a DSN carries the password).
// Over-matching ("monkey") prints [REDACTED] for a harmless value, which is the safe failure. A function,
// not a package variable, so the package holds no mutable global state. The list is the one shared with the
// logging package (doc/specs/config.md, "Default secret words").
func defaultSecretWords() []string {
	return []string{"password", "passwd", "passphrase", "secret", "token", "key", "credential", "private", "dsn",
		"authorization", "cookie"}
}

// Values is a printable view of chosen keys of a Lookup, for startup logs and diagnostics. The value of a
// secret key (WithSecrets, or a key containing one of password, passwd, passphrase, secret, token, key,
// credential, private, dsn, authorization, cookie, ignoring case and '-', '_', '.', ' ') is dropped when
// Collect builds the Values and never stored, so no printing path can show it: String, Format (%v, %+v, %#v, %s, %q), GoString, slog (LogValue) and reflection-based
// dumps all see "[REDACTED]". Unset keys print as (unset). Values is immutable and safe for concurrent use;
// the zero value is an empty view that prints as "".
//
// ID:          TB-CONFIG-013
// Pattern:     redaction of secrets in configuration output
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config" (credentials never in the codebase); pkg.go.dev fmt, "type Formatter" and "type GoStringer"; pkg.go.dev log/slog, "type LogValuer", go1.27
// Tags:        config, redaction, secrets, logging, fmt
type Values struct {
	entries []entry
}

type entry struct {
	key    string
	val    string // empty for a secret: the secret is never held
	set    bool
	secret bool
}

// Collect reads keys from l, in the order given, into a Values whose secret values are already replaced.
// A nil l gives every key as unset.
//
// ID:          TB-CONFIG-014
// Pattern:     redaction of secrets in configuration output
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config"
// Tags:        config, redaction, secrets, constructor
func Collect(l Lookup, keys []string, opts ...Option) Values {
	var s settings
	for _, o := range opts {
		if o != nil {
			o(&s)
		}
	}
	es := make([]entry, 0, len(keys))
	for _, k := range keys {
		v, ok := get(l, k)
		e := entry{key: k, set: ok, secret: s.isSecret(k)}
		if !e.secret {
			e.val = v
		}
		es = append(es, e)
	}
	return Values{entries: es}
}

func (e entry) shown() string {
	switch {
	case !e.set:
		return "(unset)"
	case e.secret:
		return redacted
	}
	return e.val
}

// String returns "key=value" pairs separated by spaces, in Collect order, each value quoted with
// strconv.Quote; a secret prints as [REDACTED] and an unset key as (unset), both unquoted.
func (v Values) String() string {
	var b strings.Builder
	for i, e := range v.entries {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(e.key)
		b.WriteByte('=')
		if e.set && !e.secret {
			b.WriteString(strconv.Quote(e.val))
		} else {
			b.WriteString(e.shown())
		}
	}
	return b.String()
}

// GoString returns a Go-syntax-like form for %#v: config.Values{"db.host": "x", "db.password": "[REDACTED]"}.
func (v Values) GoString() string {
	var b strings.Builder
	b.WriteString("config.Values{")
	for i, e := range v.entries {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(e.key))
		b.WriteString(": ")
		b.WriteString(strconv.Quote(e.shown()))
	}
	b.WriteByte('}')
	return b.String()
}

// Format implements fmt.Formatter so every verb goes through the redacted forms: %#v is GoString, %q
// quotes String, every other verb prints String.
func (v Values) Format(f fmt.State, verb rune) {
	switch {
	case verb == 'v' && f.Flag('#'):
		_, _ = fmt.Fprint(f, v.GoString())
	case verb == 'q':
		_, _ = fmt.Fprint(f, strconv.Quote(v.String()))
	default:
		_, _ = fmt.Fprint(f, v.String())
	}
}

// LogValue implements slog.LogValuer: a group with one string attribute per key, redacted as String.
func (v Values) LogValue() slog.Value {
	attrs := make([]slog.Attr, 0, len(v.entries))
	for _, e := range v.entries {
		attrs = append(attrs, slog.String(e.key, e.shown()))
	}
	return slog.GroupValue(attrs...)
}

// Option configures Collect. A nil Option is ignored.
//
// ID:          TB-CONFIG-015
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        options, configuration
type Option func(*settings)

type settings struct {
	secrets []string // lowercased exact keys
}

// WithSecrets marks keys as secret in addition to the default words (password, passwd, passphrase, secret,
// token, key, credential, private, dsn, authorization, cookie). Keys match exactly, ignoring case ("database.url" for a URL that carries a
// password). There is no option that un-marks a default word: a value is shown only when nothing marks it.
//
// ID:          TB-CONFIG-016
// Pattern:     redaction of secrets in configuration output
// Authority:   Wiggins, The Twelve-Factor App, 2011, "III. Config"
// Tags:        config, redaction, secrets, options
func WithSecrets(keys ...string) Option {
	lower := make([]string, len(keys))
	for i, k := range keys {
		lower[i] = strings.ToLower(k)
	}
	return func(s *settings) { s.secrets = append(s.secrets, lower...) }
}

func (s *settings) isSecret(key string) bool {
	k := strings.ToLower(key)
	norm := strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(k)
	for _, w := range defaultSecretWords() {
		if strings.Contains(norm, w) {
			return true
		}
	}
	for _, sk := range s.secrets {
		if k == sk {
			return true
		}
	}
	return false
}
