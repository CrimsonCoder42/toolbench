# Request: the house logging format and standard (2026-11-13)

EDITS CODE. Risk: NORMAL (additions to the logging package; no released signature changes).
Read-only references for THIS run only: advo_production/advomedix-v2/backendAdvo/util/logger.go;
advomedix-v2/.claude/CLAUDE.md lines 96-101 ("## Logging"); backendAdvo/CLAUDE.md "## Logging -- Trace logging";
red_lite_cache/internal/logger/logger.go. Nothing else outside the repo; never any .env, app.env, accounts.json.
A choice nobody made follows util/logger.go's observable output and goes into doc/ASSUMPTIONS.md.

WHY: Devin's logging display (backendAdvo util/logger.go) is the house standard for every project; it lives in
toolbench so a project imports logging instead of copying util/logger.go. log/slog is the logger; toolbench ships
handlers.

THE STANDARD (re-verify each claim against the files; cite file:line in the spec):
S1 line shape `LEVEL: 2006/01/02 15:04:05.000000 [MODULE] message | key=value | key=value`; LEVEL padded to one
   width; timestamp as log.Ldate|Ltime|Lmicroseconds; [MODULE] = slog attr "module" (or the logger's group),
   upper-case as given, never invented; attrs ` | key=value` in written order; missing/nil/empty -> `absent`;
   groups flatten to `group.key`.
S2 source `file.go:line` after the timestamp on WARN and ERROR only; an `err` attr renders last ` | err=<message>`.
S3 colour on the LEVEL prefix only (blue INFO, yellow WARN, red ERROR, DEBUG none), util/logger.go's ANSI codes,
   only with WithColor(true).
S4 trace `[TAG][STEP] message | k=v` at Info via a helper; snake_case keys; unpaired trailing key -> `absent`.
S5 levels per CLAUDE.md:98; ParseLevel accepts debug|info|warn|warning|error, unknown -> info.
S6 IDs only (CLAUDE.md:100): Redact decides what is safe; the house stack is Redact(Context(Text(...))).
S7 JSON (slog.NewJSONHandler) is the production form, text the human form; WithJSON picks; attrs and redaction
   identical.

BUILD: logging.Text(w, opts ...TextOption) slog.Handler (WithLevel, WithColor, WithSource (default warn+),
WithTimeFormat, WithClock; full slog.Handler; one mutex around the write); logging.Trace(l, tag, step, msg, kv...);
logging.ParseLevel(string) slog.Level; logging.New(w, level, opts...) *slog.Logger (Redact -> Context -> Text or
JSON). Doc comments in the exact shape; Authority cites pkg.go.dev log/slog and the slog handler guide; the spec
records the format's origin (backendAdvo util/logger.go:<lines>) as a design decision with a D-id. Golden-string
tests for every S1-S4 rule (Recorder + fixed clock, exact bytes incl. ANSI on/off); a -race concurrency test; a
secret given to New never appears in text or JSON; Example with // Output: for the plain line. Spec "House format"
section with S1-S7 and file:line evidence; doc.go; INDEX regenerated; taxonomy unchanged. doc/standards/logging.md
(new folder, indexed in doc/README.md): S1-S7 as the rule every project follows, the import line, the one-call
setup, the CLAUDE.md:100 list verbatim as a ban, and the .claude/rules lines for Devin under LEFT FOR DEVIN.

VERIFY: go test -race -timeout 20m ./...; reviewer on logging only, plus: render through logging.New with the fixed
clock and compare byte-for-byte to what util/logger.go would print (derived from source, not run):
  Info  module=AUTH_USER msg="ListAuthUsers SUCCESS" tenant_id=e1c6acea active=0 pending=0
  Trace tag=CART-CHECKOUT-PROBE step=B2 msg="cart loaded" items=3 user_id=(empty)
  Error module=FEED msg="drug names unavailable" err=<some error>
Differences are findings; the house format wins where util/logger.go is inconsistent with itself, and the spec says
which line decided it.

NOT IN THIS RUN: changes to backendAdvo or red_lite; file writers, rotation, shipping; any other package; .claude.

FINAL REPORT (<=30 lines): exports (IDs, file:line) · the three rendered sample lines verbatim · golden tests count ·
test and -race lines · counts · reviewer findings fixed / open · D-ids and assumptions · the .claude lines for Devin ·
FOUND, NOT FIXED · PIDs.
