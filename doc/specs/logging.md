# logging: slog handler wrappers (redact, context attributes, fan-out, a test recorder)

Status: SHIPPED (2026-11-13)
Package: `logging` · SHORT: `LOG` · Layer: core · Tier: T4
Work folder: `doc/wave6_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #33
Builds on: `ctxkey` (FromKey), `panics` (a panicking extract func) · Decisions: D-006 (LADDER §9) · Assumptions: see "For the lead"

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-LOG-001` | `Redact` | `func Redact(h slog.Handler, opts ...RedactOption) slog.Handler` |
| `TB-LOG-002` | `RedactOption` | `type RedactOption func(*redactConfig)` |
| `TB-LOG-003` | `WithKeys` | `func WithKeys(keys ...string) RedactOption` |
| `TB-LOG-004` | `WithPatterns` | `func WithPatterns(res ...*regexp.Regexp) RedactOption` |
| `TB-LOG-005` | `WithMask` | `func WithMask(mask string) RedactOption` |
| `TB-LOG-006` | `Context` | `func Context(h slog.Handler, extract func(context.Context) []slog.Attr) slog.Handler` |
| `TB-LOG-007` | `FromKey` | `func FromKey[T any](name string, k *ctxkey.Key[T]) func(context.Context) []slog.Attr` |
| `TB-LOG-008` | `Combine` | `func Combine(extracts ...func(context.Context) []slog.Attr) func(context.Context) []slog.Attr` |
| `TB-LOG-009` | `Multi` | `func Multi(hs ...slog.Handler) slog.Handler` |
| `TB-LOG-010` | `Recorder` | `type Recorder struct{ /* mu sync.Mutex; records []slog.Record */ }` + field `Limit` + `Enabled`, `Handle`, `WithAttrs`, `WithGroup`, `Records`, `Dropped`, `Reset` |

Options of Redact (functional, `With<Thing>`):

| Option | Default | What it sets |
|---|---|---|
| `WithKeys(keys...)` | none (the default secret words below are always on) | exact keys whose value is masked |
| `WithPatterns(res...)` | none | value patterns: every match in any text the record carries is masked |
| `WithMask(s)` | `"[REDACTED]"` | the replacement text |

Default secret words (lead decision: identical in `config` and `logging`): a key whose normalised
form CONTAINS `password passwd passphrase secret token key credential private dsn authorization cookie` is masked (substring, like
config; over-matching such as `token_count` or `monkey` is accepted). `WithKeys` keys match EXACTLY after
normalising. Authorization headers and cookies carry credentials, so they are default words
(`TestRedact_AuthorizationAndCookieMaskedByDefault`). Sentinel errors: none of our own; a panicking extract
func gives `panics.ErrPanic` from `Handle`.

## 1a. The references (read only, never run)

- `_reference/code-heim/go_19_custom_logger/logger/logger.go`: a package-global `*Logger` built in `init`
  (logger.go:22-31, writes to os.Stdout), `SetLevel` writes `logger.Level` with no lock while `Info` reads it
  (logger.go:34-36, :40): a data race. Defects -> tests: `TestDefect_go19_SetLevelRace` (the level is a
  `slog.LevelVar` the caller owns; our wrappers delegate `Enabled`, so a concurrent `Set` under `-race` is
  clean and takes effect), `TestDefect_go19_NoGlobalsNoInit` (parses this package's embedded sources: no
  `func init`, no package-level `var`, no `slog.SetDefault` call; `slog.Default()` unchanged by every test).
- `_reference/code-heim/go_26_slog/masking_info.go`: a `ReplaceAttr` that masks the keys `password` and
  `channel`; then `slog.SetDefault`. Defect: only the key is checked, so the same secret under any other key,
  inside an error message or inside a `LogValuer` result goes out unmasked; `ReplaceAttr` is never called for
  a group attribute itself (pkg.go.dev), so a group named `password` is not masked either. Test:
  `TestDefect_go26_ValueCarriesSecretPastKeyMask` shows the go_26 ReplaceAttr leaking and `Redact` with a
  pattern not leaking. `multi_write.go`: `io.MultiWriter(file, os.Stderr)` (one format, one level for both,
  a file) -> `Multi` over two handlers (each its own level), tests on `bytes.Buffer`/`Recorder` only.
  `hide_info_in_model.go`: `LogValuer` hiding a password (the right tool for a type you own; `Redact` covers
  types you do not own).
- `_reference/code-heim/go_79_loki_grafana/sample_app/main.go:58`: `slog.SetDefault` (fine in `main`, wrong in
  a library). This package never calls it (asserted by the AST test).
- Outdated (LADDER §7): per-level `log.Logger` and init-built globals -> `log/slog`; `io.MultiWriter` fan-out ->
  `slog.NewMultiHandler` (go1.26).

## 1b. The authority (opened)

- Opened: https://pkg.go.dev/log/slog, 2026-11-13 (page shows go1.27.4). `type Handler`: Handle rules ("If
  r.Time is the zero time, ignore the time", "Attr's values should be resolved", "If an Attr's key and value
  are both the zero value, ignore the Attr", "If a group's key is empty, inline the group's Attrs", "If a group
  has no Attrs, ignore it"); `HandlerOptions.ReplaceAttr`: "never called for Group attributes, only their
  contents", value already resolved; `LogValuer`, `Value.Resolve`; `Record.Clone` ("no shared state");
  `NewMultiHandler` (go1.26): "Enabled ... reports whether any of the handlers' Enabled methods return true.
  Its Handle, WithAttrs and WithGroup methods call the corresponding method on each of the enabled handlers."
  Source read: `$(go env GOROOT)/src/log/slog/multi_handler.go` (go1.27.3): Handle passes `r.Clone()` to each
  enabled handler and returns `errors.Join` of their errors; a nil handler in the list panics in Enabled.
- Opened: https://github.com/golang/example/blob/master/slog-handler-guide/README.md, 2026-11-13. Go team, "A
  Guide to Writing slog Handlers": WithAttrs/WithGroup return a new handler, never modify the receiver;
  `WithGroup("")` returns the receiver; order of `WithGroup`/`With` calls is kept (`groupOrAttrs`); resolve
  every value first; a handler that passes a record on more than once uses `Record.Clone`; clones share one
  mutex through a pointer.
- Opened: https://go.dev/blog/slog, 2026-11-13. Amsterdam, "Structured Logging with slog", The Go Blog, 2023:
  frontend `Logger`, backend `Handler`; "it can wrap another handler to add functionality"; `LogValue` to
  "redact sensitive data"; context-taking methods so "a handler can extract context information like trace IDs".
- Agreement: the authorities agree; the repos disagree (key-only masking, globals) and lose.

## 1c. Today's Go (GO_RELEASES.md)

go1.21 `log/slog`; go1.24 `slog.DiscardHandler`; go1.25 `slog.GroupAttrs`; go1.26 `slog.NewMultiHandler`; go1.27
nothing new for handlers. Still missing from the standard library: a redacting wrapper that covers groups,
`WithAttrs` and values (ReplaceAttr is per output handler, sees no group attrs and is key-oriented), a handler that
pulls context values into every record, and a concurrency-safe capturing handler for tests. `Multi` is the
one thin export (see below).

## 1d. calcbench

None. No mathematics.

## 1e. Per export

### `TB-LOG-001` `Redact` (+ 002-005 options)
- **Pattern**: decorator (wrapping slog.Handler) for log redaction / data masking.
- **Familiar shape**: a handler wrapper like the guide's `LevelHandler`; options are functional. Deviation
  from LADDER §4 `Redact(h, keys ...string)`: options, because value patterns and the mask are needed too
  (BRIEF B4 asked for `opts ...RedactOption`).
- **Key matching**: keys are normalised (lower-case, `-`, `_`, `.` and space removed). A key is secret when it
  contains a default word or equals a WithKeys key: `Password`, `api-key`, `API_KEY`, `db.dsn` all match. It
  applies to the attribute's own key at every depth: inside `slog.Group`, inside a `LogValuer` result that is a
  group, in `WithAttrs`. A matching key masks the whole value, a group's included. A `WithGroup(name)` whose name
  matches masks the value of every attribute logged under that group (record and later `WithAttrs`). Dotted
  paths are not matched as paths.
- **Value patterns**: each `*regexp.Regexp` (RE2, linear time) is applied with `ReplaceAllLiteralString` to:
  the message, every key, every group name given to `WithGroup`, every string value, and every `KindAny`
  value's renderings: `%v`, `%+v`, `encoding.TextMarshaler`, `json.Marshal`, and a `[]byte` as text. If any
  rendering matches, the value becomes the first matching rendering (order: `%+v`, `%v`, []byte, MarshalText,
  JSON) with the matches masked (so the secret is gone from
  every handler's output, JSON or text); if none matches, the value is passed unchanged (type kept). A panic
  in `MarshalText`/`MarshalJSON` is caught through `panics.TryValue` and that rendering skipped. Cost: with
  patterns set, every `KindAny` value is formatted up to four times per record.
- **Resolution**: every value is `Resolve`d (LogValuer) before the rules, recursively in groups, so a
  `LogValuer` that returns a secret is caught by key or pattern. `Value.Resolve` itself recovers a panicking
  `LogValue`.
- **WithAttrs/WithGroup**: `WithAttrs` redacts the attrs then calls the wrapped handler's `WithAttrs` (so
  pre-formatting downstream still works and secrets never reach it); `WithGroup("")` returns the receiver;
  `WithGroup(name)` calls the wrapped `WithGroup`. Neither modifies the receiver.
- **Enabled**: delegated to the wrapped handler (a `LevelVar` the caller changes takes effect at once).
- **Handle**: builds a new `Record` (same time, level, PC; redacted message; redacted attrs) and passes it on;
  the caller's record is not modified. Empty attrs and groups are passed as is (the downstream drops them).
- **Goroutines**: none. **Blocking**: only what the wrapped handler does. **Errors**: the wrapped handler's.
- **Zero value / nil**: `Redact(nil)` wraps `slog.DiscardHandler`; nil regexps are skipped; an empty mask is
  allowed (`WithMask("")`). Safe for concurrent use (immutable after construction).
- **Worked example**: `ExampleRedact` (JSON on a bytes.Buffer, time removed).

### `TB-LOG-006` `Context` (+ 007 FromKey, 008 Combine)
- **Pattern**: decorator adding context attributes (request-scoped values in every record).
- **Where the attrs go**: at the top level of the record, before the logger's `With` attrs and before the
  record's own attrs, whatever groups the logger opened, so `request_id` is always at the same path. To do
  that the wrapper keeps the `WithAttrs`/`WithGroup` chain (the guide's groupOrAttrs) and a pre-applied copy of
  the wrapped handler; on a record with context attrs it applies `WithAttrs(ctxAttrs)` to the wrapped handler and
  replays the chain (cost: the chain is re-applied for such records; with an empty chain the attrs are put first
  in a new record instead).
- **Redaction order**: `Context(Redact(h), extract)` so extracted values pass through Redact (asserted). The
  other order, `Redact(Context(h))`, adds the context attrs after Redact ran and so does NOT redact them (stated in
  the doc comment and doc.go; not tested, so no test output ever carries a secret).
- **Enabled**: delegated. **Errors**: the wrapped handler's; a panicking `extract` is recovered through
  `panics.TryValue`, the record is still passed on without context attrs, and Handle returns the joined
  `ErrPanic`. A nil ctx is `context.Background()`; a nil `extract` returns h unchanged; nil h is DiscardHandler.
- **FromKey**: reads one `ctxkey.Key[T]` and gives `slog.Any(name, v)` when present (a `LogValuer` v resolves
  downstream); a nil key gives nothing. **Combine**: concatenates extract funcs in order, skipping nil ones.
- **Goroutines**: none. **Worked example**: `ExampleContext`.

### `TB-LOG-009` `Multi`
- **Decision: wrap `slog.NewMultiHandler`, do not re-implement.** The stdlib already clones the record per
  handler, asks each handler's `Enabled`, and joins errors. The wrapper adds only the toolbench input rule:
  nil handlers are dropped (stdlib panics on a nil one); no handlers -> a handler that is never enabled. Kept as
  an export so the index finds "log fan-out" and the go_26 MultiWriter defect has a named replacement; a
  caller on go1.26+ may call `slog.NewMultiHandler` directly with the same result. A panic in one handler is not
  recovered (same as stdlib; handlers are not workers).

### `TB-LOG-010` `Recorder`
- **Pattern**: test double (recording handler; Meszaros, xUnit Test Patterns, 2007, "Test Spy").
- **Bound (rule 4)**: field `Limit` (0 or less: 10000) caps stored records; later records are refused and
  counted (`Dropped()`), never silently lost; `Reset` clears both.
- **Zero value**: ready (`var r logging.Recorder; slog.New(&r)`); must not be copied after first use (it holds
  a `sync.Mutex`; vet copylocks). All methods are safe for concurrent use; handlers made by `WithAttrs`/
  `WithGroup` write into the same Recorder.
- **What is stored**: each record with every value resolved (recursively) and the handler's `WithAttrs`/
  `WithGroup` chain folded in (`With` attrs first, then the record's attrs inside the open groups; a group left
  empty is dropped), so a test sees what an output handler would print. `Enabled` is true at every level.
  `Records()` returns clones in arrival order; `Reset()` empties it. A nil `*Recorder` is never enabled.
- Empty attrs and groups left empty are dropped at capture (Handler rules).
- Conformance: `testing/slogtest.Run` passes on `Recorder`, `Redact`, `Context`, `Multi`.

## Tests

`TestMain` with `leak.Main`. slogtest conformance for all four handlers; Redact: keys (case/normalised, nested
groups, WithAttrs, WithGroup name, group value), patterns (message, string, error, Stringer, struct by JSON
through a pointer, []byte, LogValuer result, key), mask option, nil handler, Enabled delegation, caller's record
untouched; `TestSecret_NeverInAnyOutput`: one secret through every path into JSON, text and Recorder outputs,
searched in full text; `TestExamples_NoSecretInOutput` scans the embedded example sources' `// Output:` blocks;
Context: ctxkey values in every record at root under groups/With, panic extract -> ErrPanic, nil ctx; Multi:
fan-out, per-handler Enabled, nil dropped, errors joined; Recorder: 8 goroutines x 200 records under -race,
WithAttrs/WithGroup fold, Reset; the go_19, go_26 defects. No file, env, clock or `slog.SetDefault`.

## Out of scope

Output handlers (JSON/text are stdlib), file rotation and shipping (Loki etc.: an adapter or the caller),
sampling and rate limiting of logs, a logger type of our own, `AddSource` handling, context-dependent levels.

## House format (TB-LOG-011..021, 2026-11-13)

Request: `doc/requests/logging_house_2026-11-13.md`. Origin of the format (a design decision, not an authority):
backendAdvo `util/logger.go`, advomedix-v2 `.claude/CLAUDE.md:96-101` ("## Logging"), backendAdvo `CLAUDE.md:80-103`
("## Logging — Trace logging"); red_lite `internal/logger/logger.go` for ParseLevel and the slog New shape. All read
only, never run. Authority for the code: pkg.go.dev log/slog ("type Handler" Handle rules, "type HandlerOptions"
AddSource/ReplaceAttr, "type Level" String, "Wrapping output methods"; opened https://pkg.go.dev/log/slog,
2026-11-13) and the Go team's "A Guide to Writing slog Handlers" (Handle rules, WithGroup/WithAttrs preformatting,
a shared `*sync.Mutex` around the write, pooled `*[]byte` buffers, small buffers only; opened
https://github.com/golang/example/blob/master/slog-handler-guide/README.md, 2026-11-13). WithColor also cites
ECMA-48 SGR (not opened). The Go 1.27 source confirms the JSON handler omits an empty source
(`log/slog/handler.go:491-496`).

### Exports

| ID | Signature | File |
|---|---|---|
| TB-LOG-011 | `func Text(w io.Writer, opts ...TextOption) slog.Handler` | logging/text.go:159 |
| TB-LOG-012 | `type TextOption func(*textConfig)` | logging/text.go:62 |
| TB-LOG-013 | `func WithLevel(l slog.Leveler) TextOption` (default Info; a `*slog.LevelVar` is race-free) | logging/text.go:71 |
| TB-LOG-014 | `func WithColor(on bool) TextOption` (default off) | logging/text.go:86 |
| TB-LOG-015 | `func WithSource(min slog.Level) TextOption` (default `slog.LevelWarn`; above Error = never) | logging/text.go:98 |
| TB-LOG-016 | `func WithTimeFormat(layout string) TextOption` (default `2006/01/02 15:04:05.000000`; "" = none) | logging/text.go:109 |
| TB-LOG-017 | `func WithClock(c clock.Clock) TextOption` (default: the record's time) | logging/text.go:120 |
| TB-LOG-018 | `func WithJSON(on bool) TextOption` (read by New only) | logging/text.go:131 |
| TB-LOG-019 | `func Trace(l *slog.Logger, tag, step, msg string, kv ...any)` | logging/trace.go:25 |
| TB-LOG-020 | `func ParseLevel(s string) slog.Level` | logging/level.go:16 |
| TB-LOG-021 | `func New(w io.Writer, level slog.Level, opts ...TextOption) *slog.Logger` | logging/new.go:22 |

No goroutines; nothing blocks except the shared write mutex and `w.Write`; the only error is w's. No globals, no
`init`, no `time.Now`, no `os` (TestDefect_go19_NoGlobalsNoInit scans every non-test source).

### The rules, each re-verified (file:line evidence; "source" = backendAdvo util/logger.go)

**S1 line shape.** Built: `LEVEL: 2006/01/02 15:04:05.000000 MODULE: message | key=value | group.key=value`.
- Prefix: source :35, :37, :39 write `INFO: `, `WARN: `, `ERROR: ` as the `log.New` prefix, so the line starts
  with the level, a colon and a space. **Differs from the request**: the request says "LEVEL padded to one
  width"; the source pads nothing (`INFO: ` is 6 bytes, `ERROR: ` 7). The source's output wins: unpadded.
  DEBUG (no source line: source :16-18 has three levels; CLAUDE.md:98 has four) prints `DEBUG: `. A level between
  names prints slog's name (`INFO+2: `, pkg.go.dev log/slog "func (Level) String").
- Timestamp: source :36, :38, :40 use `log.Ldate|log.Ltime|log.Lmicroseconds` = `2006/01/02 15:04:05.000000 `
  (local zone; no `LUTC`). Built as the time layout at text.go:18, from the record's time (local, as slog sets
  it) or WithClock's (text.go:206). The guide's rule "a zero Time is ignored": no clock and a zero time prints none.
- Module: util/logger.go has no module; the call-site shape is CLAUDE.md:99 `Module: message | user_id=… |
  tenant_id=…`. **Differs from the request**: the request says `[MODULE] message`; the house file says
  `Module: message`. Built as CLAUDE.md:99: `MODULE: ` (value of the top-level `module` attribute as given, never
  upper-cased or invented; the record's wins over a With one; else the logger's outermost group name, as the
  request's "(or the logger's group)"; text.go:216-237). This is the one rule most likely to need Devin's word:
  see "For the lead" A-1 (switching to brackets is two lines at text.go:235-236).
- Attributes: ` | key=value` in written order (With attributes first, preformatted once at WithAttrs) — source
  :99-102 (Trace) and CLAUDE.md:99. Values print as fmt `%v` (source :92, :95), unquoted and unescaped as the
  source prints them. Missing/nil/empty → `absent`: source :93-97 (`s != "" && s != "<nil>"`), applied to every
  attribute, not only Trace (text.go:391-416). Groups flatten to `group.key` (request; the source has no groups).
- Empty attributes and empty groups are dropped; an empty-key group is inlined (pkg.go.dev log/slog "type
  Handler"; the guide "Implementing Handle"); conformance by `testing/slogtest` (TestText_Slogtest).

**S2 source and err.**
- Source on WARN and ERROR only: source :38, :40 add `log.Lshortfile` to warn and error, not info (:36). The
  log package prints it as `file.go:42: ` (colon, space) after the timestamp. **Differs from the request**
  (`file.go:line` with no trailing colon): the source's output wins, `file.go:42: `. **util/logger.go is
  inconsistent with itself here**: Lshortfile is computed at calldepth 2 from `Printf`, which is called inside the
  wrapper, so every WARN/ERROR line names the wrapper's own line (source :113, :115, :124, :126, :135, :138,
  :145), never the caller that Lshortfile at :38/:40 exists to show. The house format wins: the record's PC, the
  real caller (text.go:213-215, :345-361), decided by the intent of :38/:40.
- `err` last as ` | err=<message>`: **util/logger.go is inconsistent with itself**: ErrorWithError (:135, :138)
  prints `message: err`, while Trace (:99-102) and CLAUDE.md:99 use ` | key=value`. The house format wins, decided
  by :99-102: a top-level `err` attribute is moved to the end of the line (With errs, then the record's;
  text.go:247-254). An `err` inside a group stays in place as `group.err`.

**S3 colour.** Source :35, :37, :39: `\033[34m` (blue) INFO, `\033[33m` (yellow) WARN, `\033[31m` (red) ERROR,
reset `\033[0m`; the codes wrap the whole prefix including `: `, so the line is
`\033[34mINFO: \033[0m2026/...`. DEBUG none. Built at text.go:26-29, :323-342, only with WithColor(true); off by
default (the source always colours: the request's "only with WithColor(true)" is a new rule, not a conflict).

**S4 trace.** Source :80-107 and backendAdvo CLAUDE.md:87-96: message `[TAG][STEP] message` (:85-90), pairs as
` | key=value` (:99-102), key `%v` (:92), unpaired trailing key → `absent` (:93), nil/empty → `absent` (:95),
logged at Info (:81, :106). Built at trace.go: the record is built as slog's "Wrapping output methods" shows (the caller's PC); the pairs become real attributes (not text in the message),
so Redact masks a secret key and the JSON form keeps the types; the text form prints the same bytes. Unpaired →
a nil value (text `absent`, JSON `null`). Keys are snake_case by the rule (CLAUDE.md:95); Trace does not change them.

**S5 levels.** CLAUDE.md:98: Debug dev-only tracing · Info a prod-worthy state change · Warn a handled failure (4xx,
retry, fallback) · Error unhandled or data-at-risk. ParseLevel accepts debug|info|warn|warning|error, unknown →
info. Source :155-165 is case-sensitive and has no debug; red_lite logger.go:47-58 lower-cases and has debug.
Built as red_lite plus TrimSpace (level.go:16-27).

**S6 IDs only.** CLAUDE.md:100 (the ban, verbatim in doc/standards/logging.md). New is the house stack: Redact (default
secret words) → Context (no extractors) → Text or JSON. Built as `Context(Redact(out), nil)` (new.go:30), the order
D-039 documents; with no extractor Context returns its handler unchanged (context.go:324-326), so it is exactly the
request's `Redact(Context(Text))`. Redact is the safety net, not permission: values that are not under a secret key
(an email under `user`) are printed; the ban is the rule.

**S7 JSON.** WithJSON(true) makes New use `slog.NewJSONHandler` (new.go:36-58) with the same Redact, the same level,
`AddSource` gated by a wrapper that zeroes the PC below the WithSource level (`sourceGate`), and WithClock's time via
ReplaceAttr. red_lite logger.go:39-45 is the same choice (JSON or text, one level). Attributes and masks are
identical (TestNew_S7_TextAndJSONSameAttrs); the module stays an ordinary `module` attribute in JSON.

### Not carried from util/logger.go

Global logger built in `init` (:29-42) and the unsynchronised `SetLevel` (:45-47): no globals; WithLevel takes a
`*slog.LevelVar`. `Fatal` (:144-147, `os.Exit` from a logger): a library never exits. `Printf`-style formats
(:50-57): slog attributes instead.

### Tests (house format)

Golden strings with a fixed clock.Fake (exact bytes): text_test.go 18 (S1 x13, S2 x3, S3 x1, S1 LogValuer, derived
handlers), trace_test.go 5 (S4, incl. source = the caller), new_test.go TestNew_RequestSamples (the request's three lines through New);
TestText_S3_NoColorByDefault (no ESC byte); TestParseLevel_S5_Table (15 rows); TestNew_S6_SecretNeverInTextOrJSON
(text and JSON, six masks each, failures print lengths only); TestNew_S7_TextAndJSONSameAttrs, TestNew_S7_JSONTimeAndSource;
TestText_Slogtest (conformance); TestText_ConcurrentUnderRace (8 goroutines x 200 lines through derived handlers,
leak.Verify); BenchmarkText; 11 Examples with `// Output:` in example_test.go (scanned by TestExamples_NoSecretInOutput).
Source lines in goldens come from `runtime.Caller` in the test (`here()`), not hard-coded numbers.

## For the lead

- INDEX rows TB-LOG-001..010; taxonomy LOG exists (LADDER §5 line 111).
- DECISIONS: Multi wraps `slog.NewMultiHandler` (alternative: own fan-out; lost: duplicates stdlib). Redact takes
  options, not `keys ...string` (LADDER §4 deviation). Context attrs at root, not in the open group (alternative:
  `r.AddAttrs` inside the open group; lost: the path of `request_id` would depend on the caller's groups).
- ASSUMPTIONS: default secret words identical to config (lead decision), substring match; WithKeys exact; mask
  `"[REDACTED]"`; Recorder Limit default 10000.
- Failure messages never print captured output: `withheld(s)` (main_test.go) prints only its length.
- Duty 2 note: `TestDefect_go26_ValueCarriesSecretPastKeyMask` asserts that the go_26 baseline (stdlib JSON handler
  + key-only ReplaceAttr, not our code) leaks into its own local buffer; that buffer is never printed on success.
  Every output of our handlers and every Example `// Output:` is searched and must be secret-free.
- doc/sources: pkg.go.dev log/slog, slog-handler-guide, go.dev/blog/slog; Meszaros (not opened).

### For the lead (house format, 2026-11-13)

- INDEX rows TB-LOG-011..021 (doc.go lists them); taxonomy unchanged.
- DECISIONS (D-id for the lead): "logging house format (TB-LOG-011..021): format: Devin's house standard, backendAdvo
  util/logger.go:35-40 (level prefixes, ANSI colours, Ldate|Ltime|Lmicroseconds, Lshortfile on WARN/ERROR), :80-107
  (Trace), :155-165 (ParseLogLevel), with advomedix-v2 CLAUDE.md:96-101 (levels, `Module: message | k=v`, IDs only).
  Text is our own slog.Handler; New = Redact → Context → Text or slog.NewJSONHandler (WithJSON). Where util/logger.go
  is inconsistent with itself the house format won: source position = the real caller (:38/:40's intent, not the
  wrapper lines :113-145), err as ` | err=` last (:99-102 over :135/:138) | alternative: slog.NewTextHandler with
  ReplaceAttr (key=value, quoted, `time=` `level=` keys) | lost: cannot print the house prefix, the module before
  the message, `absent`, or err last; projects would keep copying util/logger.go".
- ASSUMPTIONS rows (date 2026-11-13, owner logging/):
  - A-1 Module printed `MODULE: message` (CLAUDE.md:99), not the request's `[MODULE] message`. Devin to confirm;
    brackets are a two-line change at text.go:235-236 plus the goldens.
  - A-2 LEVEL unpadded (`INFO: `, `ERROR: `), as util/logger.go:35-39 prints, not the request's "padded to one width".
  - A-3 Source printed `file.go:42: ` (log.Lshortfile form) from the record's PC, not the wrapper's line.
  - A-4 Module order: record `module` > With `module` > the outermost WithGroup name; only a top-level, non-group
    `module` counts; an empty or nil module given at a level clears the module (record clears With and group,
    With clears group; reviewer finding 3, fixed; TestText_S1_EmptyModuleClears).
  - A-5 Only a top-level `err` moves last (With errs, then the record's); `error` is not special.
  - A-6 A level between names prints slog's name (`INFO+2: `) and takes the colour of the range it falls in.
  - A-7 WithClock replaces the record's time in text and JSON (in JSON it also replaces a caller's own top-level
    `time` attribute); WithTimeFormat("") prints no timestamp; no clock and a zero record time print none.
  - A-8 Trace: an unpaired key gets a nil value (text `absent`, JSON `null`). Trace builds its own record
    (slog "Wrapping output methods": runtime.Callers(2), slog.NewRecord with clock.Real().Now()) so the source
    is Trace's caller, never trace.go (reviewer finding 1, fixed; TestTrace_S4_SourceIsTheCaller).
  - A-9 ParseLevel ignores case and surrounding space (red_lite logger.go:49), unlike util/logger.go:156.
  - A-10 Values print unquoted and unescaped, as util/logger.go does; a newline in a value splits the text line
    (the JSON form escapes it). A nil writer is io.Discard; WithLevel(nil) is ignored; WithSource(slog.LevelError+4)
    turns source off; Text ignores WithJSON; JSON ignores WithColor and WithTimeFormat.
  - A-11 New builds `Context(Redact(out), nil)` (D-039's order), identical to the request's `Redact(Context(Text))`.
- CHANGELOG: logging gains the house format: Text, TextOption, WithLevel, WithColor, WithSource, WithTimeFormat,
  WithClock, WithJSON, Trace, ParseLevel, New (TB-LOG-011..021).
- doc/README.md: index doc/standards/logging.md (new folder).
- FOUND, NOT FIXED (outside this run): util/logger.go's Lshortfile names its own lines (:113-145), not the caller.

Reviewer fixes (2026-11-13): Trace records the caller's PC (finding 1), an empty module clears (3), Trace doc
comment gains Errors/Blocking (5). `go test -race -timeout 5m -count=50 ./logging/`: ok (7.124s).
REMAINING (house format): none.

Verified: `go test -race -timeout 5m -count=50 ./logging/` ok (3.584s). REMAINING: none.
