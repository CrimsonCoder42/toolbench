# Logging standard (every project)

Every Go project logs through `log/slog` with toolbench's house handler. Nobody copies backendAdvo
`util/logger.go` again: the format lives in `github.com/CrimsonCoder42/toolbench/logging`. Spec, evidence and
open assumptions: `doc/specs/logging.md` "House format".

## Setup (one call, in main)

```go
import "github.com/CrimsonCoder42/toolbench/logging"

logger := logging.New(os.Stdout, logging.ParseLevel(cfg.LogLevel))                         // text, for people
logger := logging.New(os.Stdout, logging.ParseLevel(cfg.LogLevel), logging.WithJSON(true)) // JSON, production
```

The level comes from configuration (never read the environment inside a package). Pass `logger` down (or
`logger.With("module", "FEED")` per package); never call `slog.SetDefault` from a library. A terminal can add
`logging.WithColor(true)`. Tests capture with `logging.New(&buf, slog.LevelDebug, logging.WithClock(fake))` or
`logging.Recorder`.

## The rules

**S1 Line shape.** `LEVEL: 2006/01/02 15:04:05.000000 MODULE: message | key=value | key=value`
- LEVEL is `DEBUG`, `INFO`, `WARN` or `ERROR`, followed by `: `, unpadded.
- The timestamp is date, time and microseconds (`log.Ldate|log.Ltime|log.Lmicroseconds`).
- MODULE is the `module` attribute, as given (`AUTH_USER`, `FEED`): set it once per package with
  `logger.With("module", "FEED")`. Never invented: with no module (and no group) there is none.
- Every other attribute follows as ` | key=value`, in the order written. Keys are snake_case.
- A missing, nil or empty value prints `absent`, so a missing field is visible.
- Groups flatten: `slog.Group("db", "rows", 2)` prints ` | db.rows=2`.

**S2 Source and errors.** WARN and ERROR lines carry the caller after the timestamp, `handler.go:42: `; DEBUG and
INFO do not. An error goes under the key `err` and prints last: ` | err=<message>`.

**S3 Colour.** Only on the `LEVEL: ` prefix and only with `WithColor(true)`: INFO blue `\033[34m`, WARN yellow
`\033[33m`, ERROR red `\033[31m`, reset `\033[0m`; DEBUG uncoloured.

**S4 Trace.** For an investigation: `logging.Trace(logger, "CART-CHECKOUT-PROBE", "B2", "cart loaded", "items", n,
"user_id", id)` prints `[CART-CHECKOUT-PROBE][B2] cart loaded | items=3 | user_id=absent` at INFO. TAG is
upper-case-dashed, one per investigation; STEP is `B<n>` on the backend, `M<n>` on mobile, in request-flow order;
keys snake_case; an unpaired trailing key prints `absent`. Pull one investigation out with `grep "<TAG>"`. When the
investigation ends, delete the tagged call sites; `Trace` stays.

**S5 Levels.** Debug: dev-only tracing. Info: a prod-worthy state change. Warn: a handled failure (4xx, retry,
fallback). Error: unhandled or data-at-risk (5xx, panic recovered, write or token-refresh failure).
`logging.ParseLevel` accepts `debug`, `info`, `warn`, `warning`, `error` in any case; anything else is info.

**S6 IDs only.** `logging.New` puts `Redact` over the output, so a value under a secret key (password, token, key,
secret, authorization, cookie, dsn and the rest of the shared list) prints `[REDACTED]`. That is a safety net, not
permission. The ban, verbatim from advomedix-v2 `.claude/CLAUDE.md:100`:

> IDs only. Never log email, names, DOB, phone, address, notes, any vital or measurement value, medication or symptom names, tokens, links containing a token, passwords, Authorization headers, or request/response bodies.

May log: ids, enums, booleans, counts, HTTP status, error codes and messages, paths.

**S7 Two forms, one content.** Text is the human form, JSON (`WithJSON(true)`, `slog.NewJSONHandler`) the production
form. Both carry the same attributes, the same redaction, the same level and the same source rule.

## Never

`fmt.Print*` or `log.Print*` in package code; a global logger or one built in `init`; `slog.SetDefault` in a
library; `os.Exit` from a logger; a hand-written copy of the format; a secret or personal value under any key.

## LEFT FOR DEVIN

Lines `.claude/rules` should gain (this run does not edit `.claude/`), for a new `.claude/rules/go-logging.md`
with `paths: ["**/*.go"]`, or as a section in `go-functions.md`:

```
# Logging (doc/standards/logging.md)
- Log through log/slog with the house handler: logger := logging.New(os.Stdout, logging.ParseLevel(level)); JSON in production with logging.WithJSON(true).
- One shape: `LEVEL: 2006/01/02 15:04:05.000000 MODULE: message | key=value`; module via logger.With("module", "NAME"); errors under "err" (printed last); snake_case keys.
- Investigations: logging.Trace(logger, "TAG", "B<n>", msg, kv...); delete the tagged call sites when done.
- IDs only. Never log email, names, DOB, phone, address, notes, any vital or measurement value, medication or symptom names, tokens, links containing a token, passwords, Authorization headers, or request/response bodies.
- toolbench packages never log: observability is a hook or option; never slog.SetDefault, never a global logger.
```

Two known limits for Devin to approve or fix:
- **Line forging in the text form.** Values print unescaped, as util/logger.go prints them (spec A-10), so a value
  containing "\n" starts a new line that can look like a real one in the human form; the JSON form escapes it.
  One-line fix for approval: in `logging/text.go` appendValue, escape `\n` and `\r` (as `\\n`, `\\r`) in Text only.
- **Over-masking of ID keys.** The shared secret-word rule matches substrings of the normalised key, so an ID whose
  key contains a secret word prints `[REDACTED]`: `cache_key`, `monkey_id`, `token_count` (the words "key", "token").
  Name such IDs without the word, or pass `Redact` an exact allow-list once one exists (not built).

Open before these lines are final: the module form (`MODULE: message` as CLAUDE.md:99, or the request's
`[MODULE] message`) — `doc/specs/logging.md` "For the lead" A-1.
