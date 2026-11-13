# ctxkey: a typed, collision-free key for context values

Status: SHIPPED (2026-11-13)
Package: `ctxkey` · SHORT: `CTXKEY` · Layer: core · Tier: T0
Work folder: `doc/wave1_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #4
Builds on: nothing (standard library `context`, `fmt`) · Decisions: D-006 (LADDER §9 approved, "no Must") ·
Assumptions: see "For the lead"

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-CTXKEY-001` | `Key` | `type Key[T any] struct{ /* name string */ }` + methods below |
| `TB-CTXKEY-002` | `New` | `func New[T any](name string) *Key[T]` |
| (covered by 001) | `Key.With` | `func (k *Key[T]) With(ctx context.Context, v T) context.Context` |
| (covered by 001) | `Key.Value` | `func (k *Key[T]) Value(ctx context.Context) (T, bool)` |
| (covered by 001) | `Key.String` | `func (k *Key[T]) String() string` |

No options (nothing waits, nothing is bounded). No sentinel errors (nothing fails). `New` carries its own ID
because the index rule "New<T> is covered by T" matches `NewKey`, not `New` (testing/index_test.go:339-340).

## 1a. The reference

- `_reference/code-heim/go_51_context/go_ctx_key_value/main.go` (read only, never run): stores a user id with
  `context.WithValue(ctx, "userID", 42)` and reads it back with `ctx.Value("userID").(int)` in two functions.
- **Defect (becomes a test)**: the key is a plain `string`. Any other package that also uses the string
  `"userID"` as a key reads or overwrites the same slot; the type assertion silently returns `ok == false`
  when that other package stored a different type. This is exactly what the `context` documentation forbids.
  Test: `TestKey_StringKeyCollision` stores under the string key `"userID"` (as go_51 does) and under
  `New[int]("userID")` and shows each reads only its own value.
- Outdated (LADDER §7): "string context keys (go_51): unexported key type or `ctxkey.Key[T]`"; the repeated
  `ctx.Value(k).(T)` assertion at every read site is what generics remove.
- The other go_51 folders (cancel, deadline, timeout) are not this package; the go_51 timeout goroutine leak
  belongs to rank 29 (retry/timeout).

## 1b. The authority (opened)

- Opened: https://pkg.go.dev/context#WithValue, 2026-11-13. `func WithValue`: "The provided key must be
  comparable and should not be of type string or any other built-in type to avoid collisions between packages
  using context. Users of WithValue should define their own types for keys." `type Context`, method `Value`:
  "Value returns the value associated with this context for key, or nil if no value is associated with key";
  "A key can be any type that supports equality; packages should define keys as an unexported type to avoid
  collisions"; the same doc shows the `NewContext`/`FromContext` "type-safe accessors" this package
  generalises.
- Opened: https://go.dev/blog/context, 2026-11-13. Ajmani, "Go Concurrency Patterns: Context", The Go Blog,
  2014, section "Example: Google Web Search" (package userip): "The key type is unexported to prevent
  collisions with context keys defined in other packages", with `NewContext` and `FromContext`. Section
  "Context": values are request-scoped data that "must be safe for simultaneous use by multiple goroutines".
- Read: Go standard library source `/usr/local/go/src/context/context.go` (go1.27.3): `WithValue` (line 728)
  panics on a nil parent, a nil key and a non-comparable key; `valueCtx.String` (line 763) prints
  `<parent>.WithValue(<key>, <val>)` using the key's and value's `String()` if they are `fmt.Stringer`, the
  string itself for a `string`, else only the type name (`stringify`, line 751).
- Agreement: authority and repo disagree (string key); the authority wins.

## 1c. Today's Go

Nothing in the standard library or golang.org/x (go1.27, GO_RELEASES.md) offers a typed key; go1.27 generic
methods do not change that (`context.Context.Value` still takes and returns `any`). The idiom is still a
hand-written unexported type plus two accessor functions per value. toolbench
keeps the export: one line `var userID = ctxkey.New[int]("user-id")` replaces the type, the two accessors
and the type assertion.

## 1d. calcbench

None. No mathematics.

## 1e. Per export

### `TB-CTXKEY-001` `Key`
- **Pattern**: typed context key (type-safe accessor for request-scoped values).
- **Authority**: pkg.go.dev `context`, `WithValue` and `Context.Value`; Ajmani, Go Blog 2014, "Example: Google
  Web Search".
- **Familiar shape**: `NewContext(ctx, v)` / `FromContext(ctx) (v, ok)` from the userip example, as methods on
  the key: `With` / `Value`. `Value` returns `(T, bool)` like the `FromContext` and like a map read.
- **Key identity**: the key stored in the context is the `*Key[T]` pointer, so identity is the pointer: two
  `New` calls with the same name (or the same name and type) are two distinct keys and never see each other's
  values. `Key` has a `string` field, so it is never zero-size and the Go spec rule "pointers to distinct
  zero-size variables may or may not be equal" cannot make two keys equal. Two keys of different `T` are
  different pointer types and can never be equal. A `*Key[T]` is comparable, so `context.WithValue` never
  panics on it.
- **Name**: for printing only, never identity. `String()` returns the name, so `fmt.Sprint(ctx)` shows
  `context.Background.WithValue(user-id, int)`. Keys should be namespaced by the caller ("auth.user-id").
- **Stored value**: kept in an unexported box `entry[T]`, so a stored nil interface value (`With(ctx, error(nil))`)
  reads back as `(nil, true)`, distinct from "missing". The box's `String()` mirrors the standard library's
  `stringify` (Stringer, string, else the type name), so printing a context shows what plain `WithValue` would
  show and never prints a non-string value.
- **Value**: missing key → `(zero, false)`; nil ctx → `(zero, false)`; nil `*Key` → `(zero, false)`. Shadowing:
  a child `With` hides the parent's value for the child only (standard `Value` lookup walks to the root).
- **With**: nil `*Key` → returns ctx unchanged (stores nothing; documented, never a panic). nil ctx → the value
  is attached to `context.Background()` instead of panicking (the stdlib panics "cannot create context from nil
  parent"; toolbench rule: a library function never panics on input). See "For the lead".
- **Goroutines**: none. **Cancel**: none (With derives a value context; cancellation is the parent's).
  **Errors**: none. **Blocking**: none. Safe for concurrent use: a `Key` is immutable after `New`.
- **Zero value**: a `Key[T]{}` used through a pointer is a valid, distinct key with the empty name; create with
  `New` so it has a name. Keys are package-level `var`s in the consumer; copying a `Key` value makes a different
  key (the pointer is the identity).
- **No Must**: no `MustValue`; a missing value is `ok == false` (Devin, D-006).
- **calcbench IDs called**: none. **Compared against**: none (no library named).
- **Worked example**: `ExampleNew`, `ExampleKey_Value` in `ctxkey/key_test.go`.

### `TB-CTXKEY-002` `New`
- Returns a new, distinct `*Key[T]` named `name` (any string, the empty one included). Never fails.

## 1g. Comparison

None (LADDER names no library).

## 1h. Out of scope

Typed keys for `slog` attributes (logging package), request-ID generation and propagation (httpx/grpcx
middleware will build on `Key`), deadlines and cancellation (stdlib `context`).

## Tests (`ctxkey/key_test.go`)

`ExampleNew`, `ExampleKey_Value` (with `// Output:`) · `TestKey_RoundTrip` (int, string, struct, pointer,
interface with nil) · `TestKey_DistinctSameName` (same name, same T; empty name) · `TestKey_Shadowing` (child
hides parent, parent unchanged) · `TestKey_Missing` · `TestKey_NilContext` · `TestKey_NilKey` ·
`TestKey_StringKeyCollision` (the go_51 defect) · `TestKey_String` (key name and `fmt.Sprint(ctx)`) ·
`TestKey_Concurrent` (many goroutines read one ctx, leak.Verify) · `BenchmarkKey_Value` with `b.ReportAllocs()`
· `TestMain` with `leak.Main`.

## For the lead

- taxonomy row exists (LADDER §3 line 91); INDEX rows TB-CTXKEY-001 `Key`, TB-CTXKEY-002 `New`.
- Decision to log (default chosen, alternative lost): `With(nil, v)` attaches to `context.Background()`
  instead of panicking like `context.WithValue`; alternative "panic like stdlib" lost to the CLAUDE.md rule
  "a library function never panics on input". `With` on a nil `*Key` returns ctx unchanged; alternative
  "store under the typed nil pointer" lost because every nil `*Key[T]` of one T would share a slot.
- Source record `doc/sources/go-blog-context.md` / `pkg-go-dev-context.md`: used by TB-CTXKEY-001, -002.

## Align (2026-11-13)

Work folder: `doc/align_2026-11-13/` · Task D6 (decided by Devin) · Builder: ctxkey · Additive only.

### Matrix row
`MATRIX.md` row 43: request id / correlation id (HTTP) · planned `httpx.RequestID` + ctxkey · REPEAT 0/4/4 ·
verdict PLANNED (D6 builds the key). List (b) rank 8: "ctxkey: standard request-id key, NewRequestID
(crypto/rand), helpers" → ctxkey, builds D6. `M4.md` row "request id / correlation id": reuse the inbound id only
after validating length and charset (an inbound header is attacker text: log forging); generate with crypto/rand
otherwise; stash with `ctxkey`; `logging.FromKey` reads it; uncovered → test "an inbound id of 10 KB or with
CR/LF is replaced, not echoed". This section ships the key, the validator, the generator and the ctx helpers;
the HTTP middleware (echo on the response, propagate on outgoing requests) stays in `httpx` (row 43) and the
message header `pubsub.HeaderRequestID = "request-id"` is the pubsub builder's (this run); ctxkey imports neither.

### References read (a reading list, never cited as authority; `_reference/`, read only)
- ak/GO-CQRS-Microservices `internal/pkg/http/echo/middleware/correlation_id_middleware.go:9-25`: reuses the
  inbound header unvalidated, else generates; stores under a plain string key (the go_51 collision, already
  `TestKey_StringKeyCollision`). Ours: `WithRequestID` validates first; `RequestID` is a `*Key[string]`.
- ak/BulletProof-DynamoDB-GO `internal/routes/routes.go:38` (chi `RequestID`: host + counter, predictable).
- ak/GO-Kafka-MongoDB-Prometheus-Grafana `internal/server/http.go:55` (echo `RequestID`: its own key, its own id).
- ak/Kafka-GO-Real-Time-Feed `producer/main.go:93` (an id put on the message, never read from a ctx).
  Four sources, four keys, four id formats: no two can be joined in a log. Ours: one key, one validator.

### Authority (`.claude/rules/sources.md`)
- Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier": the requestor
  puts a unique id on the request; every reply and every log line carries it back. **Not opened** (the book is
  not online); named for the pattern.
- pkg.go.dev `crypto/rand`, "func Text" (go1.24); opened as the installed source `$GOROOT/src/crypto/rand/text.go`
  (go1.27.3), 2026-11-13: "a cryptographically random string using the standard RFC 4648 base32 alphabet ...
  at least 128 bits of randomness ... the likelihood of collisions vanishingly small. A future version may
  return longer texts". Today 26 characters of `A-Z2-7`. `rand.go:41-46`: `Read` "never returns an error ...
  crashes the program irrecoverably" if the OS source fails. So `NewRequestID` has no error result.
- pkg.go.dev `context`, "func WithValue" (opened for TB-CTXKEY-001, relied on again): the stored key is the
  `*Key[string]` pointer, so `RequestID` collides with nothing.
- RFC 9110, HTTP Semantics, 2022, §5.5 "Field Values": "Field values containing CR, LF, or NUL characters are
  invalid and dangerous"; a field value is visible ASCII (VCHAR, 0x21-0x7E) with SP/HTAB only between tokens.
  `ValidRequestID` takes the strict form: every byte in 0x21-0x7E, so no space, no tab, no CR/LF/NUL, no DEL,
  no non-ASCII; an id passes a header, a log line and a message header unchanged. Locator for the rule only;
  the validator is ours.
- Compare-against (shape only, not opened as authority): chi `middleware.RequestID` (`GetReqID(ctx)`), echo
  `middleware.RequestID` (header reuse, `Generator` option). Ours: no middleware here, a plain key + helpers.

### Today's Go (go1.27)
`crypto/rand.Text` (1.24) is the standard-library identifier: URL- and header-safe, no version semantics, no
seeding, no global state. Alternative considered: the go1.27 standard-library `uuid` package (RFC 9562,
`uuid.New()`, 36 characters with hyphens, version/variant bits). Text won: shorter (26 vs 36), the same 128 bits
of entropy, one alphabet that passes every header and log rule above without escaping, nothing to parse; a
consumer that must emit a UUID calls `uuid.New().String()` itself and stores it with `WithRequestID` (it passes
`ValidRequestID`). calcbench has no identifier or random-string primitive (`calcbench_requests.md`, last
paragraph: ALG-SAMPLE-* draw from a caller's source and produce no identifier); an id is not mathematics, so no
request is raised and the standard library is used.

### calcbench
None.

### Exports

| ID | Name | Signature |
|---|---|---|
| `TB-CTXKEY-003` | `RequestID` | `var RequestID = New[string]("ctxkey.request-id")` |
| `TB-CTXKEY-004` | `MaxRequestIDLen` | `const MaxRequestIDLen = 128` |
| `TB-CTXKEY-005` | `ErrRequestID` | `var ErrRequestID = errors.New("ctxkey: invalid request id")` |
| `TB-CTXKEY-006` | `NewRequestID` | `func NewRequestID() string` |
| `TB-CTXKEY-007` | `ValidRequestID` | `func ValidRequestID(s string) bool` |
| `TB-CTXKEY-008` | `WithRequestID` | `func WithRequestID(ctx context.Context, id string) (context.Context, error)` |
| `TB-CTXKEY-009` | `RequestIDFrom` | `func RequestIDFrom(ctx context.Context) (string, bool)` |
| `TB-CTXKEY-010` | `EnsureRequestID` | `func EnsureRequestID(ctx context.Context) (context.Context, string)` |

All in `ctxkey/request_id.go`. Goroutines: none. Cancel: none (nothing waits). Blocking: none. Nothing panics
on input. No global mutable state (`RequestID` is an immutable key; `ErrRequestID` an immutable sentinel).

#### `TB-CTXKEY-003` `RequestID`
- **Pattern**: Correlation Identifier (EIP), carried as a typed context value.
- **Contract**: the one `*Key[string]` every toolbench package reads and writes for the request / correlation
  id: `logging.FromKey(RequestID, "request_id")`, the pubsub `request-id` header, later `httpx`/`grpcx`. A
  request id is an opaque string, 1..`MaxRequestIDLen` bytes, every byte printable ASCII 0x21-0x7E. Being a
  `Key`, `RequestID.With` stores any string unvalidated; the helpers below are the validated path and
  `EnsureRequestID` replaces a stored invalid id, so a consumer that bypasses the helpers is corrected at the
  next boundary. Printed name `ctxkey.request-id` (`fmt.Sprint(ctx)`).
- **Never reassigned**: `RequestID` is an exported package variable (the same shape as every `Err...` sentinel)
  because a `*Key[string]` is a pointer identity and Go has no const for one. Its value is the identity: an
  importer that assigns to it makes its own key, and every other package's `With`/`Value` then misses that
  id (the id is split between packages). Read it, never assign to it, like `io.EOF`; the doc comment says so.
- **Zero value**: n/a (a package-level variable, ready to use).

#### `TB-CTXKEY-004` `MaxRequestIDLen`
- 128 bytes. Room for a UUID (36), a W3C traceparent (55), a `host-counter` chi id, with margin; small enough
  that a stored id is never a memory or log-size concern. An inbound 10 KB header is rejected, not stored.

#### `TB-CTXKEY-005` `ErrRequestID`
- Sentinel returned (wrapped, `errors.Is`) by `WithRequestID` for an invalid id. The wrapped message names
  the reason and the byte length only, never the id's bytes: an invalid id is attacker text and the error
  message is the first thing that gets logged (log forging through the error path would defeat the validator).

#### `TB-CTXKEY-006` `NewRequestID`
- `crypto/rand.Text()`: one `Read` of 26 bytes, one 26-byte string. >= 128 bits of randomness; distinct ids
  with vanishing collision probability; always passes `ValidRequestID` (base32 alphabet is within 0x21-0x7E and
  26 <= 128). **Errors**: none; an unusable OS random source crashes the program by `crypto/rand`'s contract
  (go1.24+), never a weak or repeated id. No seed, no counter, no shared state: safe from any goroutine.

#### `TB-CTXKEY-007` `ValidRequestID`
- `true` iff `1 <= len(s) <= MaxRequestIDLen` and every byte is in 0x21..0x7E. Byte-wise (no UTF-8 decoding):
  any multi-byte rune has a byte >= 0x80 and is rejected. Pure, O(len), no allocation.

#### `TB-CTXKEY-008` `WithRequestID`
- Validates, then `RequestID.With`. Invalid: returns `(ctx, ErrRequestID wrapped)` with ctx unchanged (nothing
  stored, nothing echoed). A nil ctx is `context.Background()` on both paths (the returned ctx is then
  Background, never nil). **Errors**: `ErrRequestID` only.

#### `TB-CTXKEY-009` `RequestIDFrom`
- `RequestID.Value(ctx)`: `(id, true)` or `("", false)`; nil ctx → `("", false)`. Returns what is stored, even
  an invalid id stored by a direct `RequestID.With`; callers that forward an id to a header use
  `EnsureRequestID` or `ValidRequestID`.

#### `TB-CTXKEY-010` `EnsureRequestID`
- The existing id when one is stored and valid: `(ctx, id)` with ctx unchanged and no allocation. Otherwise
  (missing, or stored but invalid) a fresh `NewRequestID` stored in a child ctx: `(child, id)`. Never fails.
  The shape a middleware uses: `ctx, id := ctxkey.EnsureRequestID(r.Context())`.

### Scale note
- Bounded memory: yes. One id per ctx, <= 128 bytes; nothing is retained by the package.
- Contention point: none. No lock, no counter, no shared state; `crypto/rand.Read` is the kernel's source
  (`getrandom`/`arc4random_buf`), which is per-call and not serialised by this package.
- Per call: `NewRequestID` one allocation (32 B: the compiler keeps the 26-byte scratch slice on the stack and
  allocates only the result string; measured 1 alloc/op, `BenchmarkNewRequestID`, "Measured" below);
  `EnsureRequestID` on a hit zero allocations, on a miss 3 (the id string, the `valueCtx`, the boxed entry);
  `ValidRequestID` zero; `WithRequestID` one `valueCtx` (one `fmt.Errorf` on the error path).
- At 10x and 100x callers nothing changes: per-request cost is constant and nothing is shared. What breaks
  first is outside this package (the log writer, M4 "access log" row). The validator is O(len) and len is
  checked before the byte scan, so a 10 KB inbound id costs one length compare.

### Tests (`ctxkey/request_id_test.go`)
`ExampleRequestID`, `ExampleNewRequestID`, `ExampleValidRequestID`, `ExampleWithRequestID`, `ExampleRequestIDFrom`,
`ExampleEnsureRequestID`, `ExampleMaxRequestIDLen`, `ExampleErrRequestID` (each `// Output:`; the random value
is never printed) · `TestNewRequestID_uniqueAndValid` (10 000 distinct, all valid, all 26 bytes today) ·
`TestNewRequestID_concurrent` (64 goroutines, all distinct, `leak.Verify`) · `TestValidRequestID` table (empty,
128 ok, 129 too long, space, CR, LF, NUL, tab, unicode, DEL 0x7F, 0x21 and 0x7E boundaries ok, a UUID ok) ·
`TestWithRequestID_invalidLeavesCtxUnchanged` (the M4 defect, GO-CQRS correlation_id_middleware.go:9-25: a
10 KB id and a CR/LF id are rejected, ctx is the same value, nothing stored, `errors.Is(err, ErrRequestID)`) ·
`TestWithRequestID_errorNeverEchoesID` (log forging through the error path) · `TestWithRequestID_nilCtx` ·
`TestEnsureRequestID_keepsExistingAndMintsMissing` (kept: same ctx, same id; missing: minted and stored;
stored-but-invalid: replaced) · `TestRequestIDFrom_nilCtx` · `BenchmarkNewRequestID`, `BenchmarkEnsureRequestID`
(hit and miss) with `b.ReportAllocs()`.

### For the lead
- INDEX rows to regenerate: TB-CTXKEY-003..010 (`ctxkey/doc.go` lines added).
- Decision to log (default chosen, alternative lost): id format `crypto/rand.Text` (26 base32) over the go1.27
  stdlib `uuid` (RFC 9562): shorter, header/URL/log-safe with no escaping, no version semantics; a consumer
  needing a UUID stores `uuid.New().String()` through `WithRequestID`. Second: `MaxRequestIDLen = 128` over
  64 (fits traceparent 55 and UUID 36 with margin) and over 256 (no known id format needs it).
- Assumption: `WithRequestID(nil, invalid)` returns `context.Background()` (not nil) with the error, so a caller
  that ignores the error still holds a usable ctx; consistent with `Key.With(nil, v)`.
- Assumption: `RequestID` stays an exported package `var` (a pointer identity has no const form), under the
  sentinel-error convention "read, never assign"; its doc comment states it. Alternative, a `func RequestID()
  *Key[string]` accessor, lost: unfamiliar shape, and `logging.FromKey(ctxkey.RequestID, ...)` reads naturally.
- Assumption: `ValidRequestID` is strict VCHAR (rejects SP and HTAB, which RFC 9110 permits inside a field
  value): an id is one token, and a space would split it in every log line.
- Source records to add: `doc/sources/eip.md` (Correlation Identifier → TB-CTXKEY-003), `doc/sources/pkg-go-dev-crypto-rand.md`
  (Text → TB-CTXKEY-006), `doc/sources/rfc9110.md` (§5.5 → TB-CTXKEY-004/007/008).
- calcbench request: none (an id is not mathematics; `calcbench_requests.md` last paragraph already says so).

### Measured (VERIFY, 2026-11-13, go1.27.3 darwin/arm64, -race on; raw in doc/align_2026-11-13/out/bench_new.txt)
BenchmarkNewRequestID 976 ns/op, 32 B/op, 1 alloc/op; EnsureRequestID hit 111 ns/op, 0 allocs; miss 1 149 ns/op, 3 allocs (the id, the context, the entry box); ValidRequestID 27 ns/op, 0 allocs.
