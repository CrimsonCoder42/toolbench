// Package logging holds log/slog handler wrappers, never a logger of its own: Redact masks secrets by key (case
// and separator insensitive, at every group depth; a key containing password, passwd, passphrase, secret,
// token, key, credential, private, dsn, authorization or cookie always) and by value pattern (in messages, strings, error texts and
// LogValuer results), Context puts request-scoped values from ctxkey keys into every record, Multi fans a
// record out to several handlers (slog.NewMultiHandler), and Recorder captures records for tests. Nothing here
// writes a file, reads the environment, starts a goroutine or calls slog.SetDefault: the caller builds the
// output handler and the logger. Put Redact closest to the output, Context(Redact(h), extract), so extracted
// values are redacted too.
//
// The house format (doc/standards/logging.md) is Text, a handler printing
// "LEVEL: 2006/01/02 15:04:05.000000 file.go:42: MODULE: message | key=value | err=message", with Trace for
// "[TAG][STEP] message | k=v" investigation lines, ParseLevel for level settings, and New, the one-call
// setup: Redact, then Context, then Text (or slog.NewJSONHandler with WithJSON).
//
// ID short form: LOG (IDs are TB-LOG-NNN).
//
// Exports:
//
//	TB-LOG-001  Redact        wraps a handler so secret keys and value patterns are masked in every record
//	TB-LOG-002  RedactOption  configures Redact
//	TB-LOG-003  WithKeys      adds exact keys whose values are masked (the default secret words stay on)
//	TB-LOG-004  WithPatterns  masks every match of these regexps in any text the record carries
//	TB-LOG-005  WithMask      sets the replacement text (default "[REDACTED]")
//	TB-LOG-006  Context       wraps a handler so attributes taken from the context are added to every record
//	TB-LOG-007  FromKey       an extract func reading one ctxkey.Key as one attribute
//	TB-LOG-008  Combine       joins extract funcs into one, in order
//	TB-LOG-009  Multi         sends each record to every enabled handler (slog.NewMultiHandler, nils dropped)
//	TB-LOG-010  Recorder      a handler that keeps records for tests (bounded, Limit default 10000); the zero value is ready
//	TB-LOG-011  Text            the house text handler: "LEVEL: time file:line: MODULE: message | key=value", err last
//	TB-LOG-012  TextOption      configures Text and New
//	TB-LOG-013  WithLevel       sets the lowest level written (a *slog.LevelVar changes it at run time)
//	TB-LOG-014  WithColor       colours the level prefix with ANSI codes (default off)
//	TB-LOG-015  WithSource      prints file.go:line on records at this level or above (default WARN)
//	TB-LOG-016  WithTimeFormat  sets the timestamp layout (default "2006/01/02 15:04:05.000000"; "" none)
//	TB-LOG-017  WithClock       takes every timestamp from a clock.Clock (tests)
//	TB-LOG-018  WithJSON        makes New build the JSON form (slog.NewJSONHandler)
//	TB-LOG-019  Trace           logs "[TAG][STEP] message | k=v" at Info; an unpaired key is absent
//	TB-LOG-020  ParseLevel      maps debug, info, warn, warning, error (any case) to a slog.Level; else Info
//	TB-LOG-021  New             the house logger: Redact, then Context, then Text or JSON
package logging
