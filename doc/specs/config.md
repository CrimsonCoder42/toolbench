# config: layered configuration (defaults < file < environment < flags), typed getters, redaction

Status: SHIPPED (2026-11-13; reviewer findings fixed, `go test -race -timeout 5m -count=50 ./config/` ok)
Package: `config` · SHORT: `CONFIG` · Layer: core · Tier: T4
Work folder: `doc/wave6_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #31
Builds on: standard library only (`encoding/json`, `flag`, `io`, `io/fs`, `strconv`, `time`, `errors`, `maps`,
`log/slog` for `LogValuer`). Not used: `clock` (nothing waits or measures), `panics` (no caller callback runs:
a `Lookup` is called, but it is the caller's own function on the caller's goroutine, and the package recovers
nothing), `group`/`guard` (no goroutine, no shared mutable state) · Decisions/assumptions: "For the lead"

## Research

### a. The reference (code-heim; a reading list, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `go_17_env_variables/gin_blogs_after` | `godotenv.Load()` in `main.go:13`, then `os.Getenv("DB_CONN_STR")` in `models/setup.go:13`. | **Load error ignored** (`main.go:13`). **Unset = empty**: `os.Getenv` cannot tell a missing DSN from an empty one; the failure surfaces later as a `panic("Failed to connect...")`. Global `DB`. Tests `TestDefect_go17_GetenvUnsetLooksEmpty`, `TestDefect_go17_LoadErrorIgnored`. |
| `go_31_yaml_config/app_config/app_config.go` | `ReadConfig` decodes `config.yml` into the package global `Cfg` (`:21`). | **Global config** (`:21`); **panic** on a missing or bad file (`:26`, `:34`); **committed secrets** (`config.yml` carries `fb_oauth.client_secret`; values not copied here); hard-coded session key `[]byte("secret")` (`main.go:20`). Tests `TestDefect_go31_GlobalConfig`, `TestDefect_go31_PanicOnBadFile` (also searches the error for the secret). |
| `go_72_viper_config/{evn,yaml,struct}_config/config/app_config.go` | viper global: `AddConfigPath`, `AutomaticEnv`, `ReadInConfig`. | **`log.Fatal` in the loader** (`:17-19` each): a library exits the process. **`viper.Unmarshal` error ignored** (`struct_config :32`). Global `AppConfig` and global viper. **Password concatenated into a DSN** (`struct_config/models/setup.go:13-19`), the string that error logs then print. `app.env` exists in `evn_config/config/` and was NOT opened. Tests `TestDefect_go72_LogFatalInLoader`, `TestDefect_go72_UnmarshalErrorIgnored`, `TestDefect_go72_SecretLogged`. |

### b. The authority (opened 2026-11-13)

- Wiggins, The Twelve-Factor App, 2011, "III. Config". Opened: https://12factor.net/config, 2026-11-13. "An app's
  config is everything that is likely to vary between deploys"; "strict separation of config from code"; the
  litmus test "the codebase could be made open source at any moment, without compromising any credentials";
  "stores config in environment variables"; "env vars are granular controls, each fully orthogonal". Used for:
  the environment layer, per-key (not per-"environment") lookups, Required at startup, and redaction.
- pkg.go.dev `os.LookupEnv`. Opened: https://pkg.go.dev/os#LookupEnv, 2026-11-13. "If the variable is present
  in the environment the value (which may be empty) is returned and the boolean is true." `Lookup` has exactly
  this meaning; `FromEnv` takes it as an injected func. `os.DirFS` (same page): passed by callers to
  `FromJSONFile`; its doc points to `Root.FS` when symlink containment matters (we accept any `fs.FS`).
- pkg.go.dev `flag`. Opened: https://pkg.go.dev/flag, 2026-11-13. `FlagSet.Visit`: "It visits only those flags
  that have been set"; `VisitAll` visits all. `FromFlags` uses `Visit`, so a flag's default never overrides a
  lower layer.
- pkg.go.dev `encoding/json`. Opened: https://pkg.go.dev/encoding/json, 2026-11-13. `Decoder.UseNumber`
  (numbers as `Number`, the literal text), `Decoder.Token`/`More` (streaming tokens), and "In v1, a JSON object
  with duplicate names is permitted" (last wins silently): we read tokens so duplicates are detected and refused.
- pkg.go.dev `io/fs`. Opened: https://pkg.go.dev/io/fs, 2026-11-13. `FS` ("the minimum implementation"),
  `ValidPath` (unrooted slash paths; "/x" invalid), `ReadFile`. `FromJSONFile` uses `fsys.Open` and closes it.

### c. Today's Go (go1.27) and why the export is still needed

The standard library has the pieces (`os.LookupEnv`, `flag`, `encoding/json`) but no layering, no typed getters
with set/unset semantics, no "every missing key at once", no redaction. viper does all of it as a framework with
global state, reflection (`mapstructure`) and many dependencies; caarlos0/env and envconfig are struct-tag,
reflection-based and environment-only. toolbench needs a small, reflection-free, global-free layer (LADDER §4).
`errors.AsType` (go1.26) is used to unwrap `*strconv.NumError`.

### d. calcbench

None. Nothing here is mathematics (parsing is strconv/time).

## Decisions taken in this spec (each with the alternative that lost)

1. **Precedence: later wins.** `Layer(defaults, file, env, flags)` reads left to right in rising precedence,
   matching the request's "defaults < file < environment < flags". Lost: first-wins (a search path, viper's
   internal order), which forces the caller to write the order backwards from how the request states it.
2. **Set vs unset is os.LookupEnv's.** A key set to "" is set and overrides lower layers; typed getters return
   `ErrParse` for "" (it is not a number); `Required` counts "" as missing. Lost: "" = unset everywhere (viper's
   default without `AllowEmptyEnv`): it makes `KEY=` impossible to express and hides the go_17 bug instead of
   reporting it. Required treats "" as missing because an empty required value is exactly the go_17 mistake.
3. **Key naming across layers.** Canonical keys are lowercase, dotted (`db.host`, `http.read-timeout`).
   Environment: `prefix + EnvName(key)` with `EnvName` = upper case, `.` and `-` → `_` (`APP_DB_HOST`,
   `APP_HTTP_READ_TIMEOUT`); the prefix is used verbatim (include the `_`). Flags: `FlagName(key)` = `.` and `_`
   → `-` (`-db-host`, `-http-read-timeout`), case kept. File: nested objects joined with `.`
   (`{"db":{"host":…}}`), names matched exactly (no case folding). `EnvName`/`FlagName` are exported so programs
   can list the variables and flags they read. Collision stated: `db.host` and `db_host` read the same env var.
   Lost: case-insensitive file keys (silent collisions between `Host` and `host`).
4. **Typed getters** `Int`, `Duration`, `Bool` return `(def, nil)` when unset, `(zero, ErrParse)` when set but
   malformed (zero, not def: Go's "result unspecified on error" convention, so ignoring the error is visible).
   `Int` is base 10 (`strconv.ParseInt(v, 10, 0)`; "0x10", " 1", "1.5" are errors). `Duration` is
   `time.ParseDuration`. `Bool` is `strconv.ParseBool`. `String` never fails. Errors wrap `ErrParse` and
   `strconv.ErrSyntax`/`ErrRange`. **No error message contains a value**: the `*strconv.NumError` (which quotes
   the input) is unwrapped to its bare cause, and `time.ParseDuration`'s error (which quotes the input) is
   replaced by `strconv.ErrSyntax`. Lost: quoting the bad value (friendlier, but leaks a secret mistyped into a
   numeric key; the package does not know which keys are secret at that point).
5. **Required** returns `errors.Join` of one error per missing key, in argument order, duplicates once, each
   wrapping `ErrMissing` and naming the key. nil when all present.
6. **JSON flattening.** Objects join with `.`; strings as is; numbers keep their literal text (`UseNumber`:
   `12345678901234567890` and `1.5e3` survive, no float64 round trip); booleans "true"/"false"; **null = unset**
   (a lower layer applies; lost: null = "" which would override defaults with nothing); **arrays of scalars
   join with `,`** (`[]` = ""), the same form an environment variable uses (`HOSTS=a,b`), so all layers agree;
   an array holding an object, array or null, or an element containing `,`, is `ErrJSON` (lost: indexed keys
   `a.0`, which no environment layer can mirror). **Duplicate and colliding keys are `ErrJSON`**
   (`{"a":1,"a":2}`, `{"a.b":1,"a":{"b":2}}`, `{"a":{…},"a":{…}}`); lost: encoding/json v1's silent last-wins.
   Empty names, a non-object top level and trailing data are `ErrJSON`. **Bounds**: 4 MiB of input, 64 levels of
   nesting (unexported constants, documented on `FromJSON` and `ErrJSON`). A syntax error is reported as
   `ErrJSON: syntax error at byte N`, offset only: the `*json.SyntaxError` is not wrapped because its text
   quotes one input character (review NIT; lost: wrapping it for `errors.As`). A read error from the caller's
   `io.Reader` is wrapped unchanged.
7. **Redaction model: never hold the secret.** `Collect(l, keys, opts...)` builds a `Values`; for a secret key
   the value is dropped at construction, so no printing path (String, `Format` for every verb, `GoString`,
   `slog.LogValuer`, reflection dumps of unexported fields, `%+v` inside a struct) can show it. Default secret
   words: see "Default secret words" below (dsn because go_72's DSN carries the password). Over-matching (`monkey`) only hides a harmless
   value: the safe failure. `WithSecrets(keys...)` adds exact keys (case-insensitive), e.g. `database.url` for a
   URL with a password. There is no un-mark option. Lost: a `Format` method over a Values that holds the
   secret (one missed printing path — reflection, a future `MarshalJSON` — would leak it).
8. **FromFlags reads the FlagSet at each lookup** (via `Visit`), so it can be made before `fs.Parse`; lookups
   must not run concurrently with `Parse`/`Set` (FlagSet is not safe for that). Lost: snapshot at construction
   (silently empty when built before Parse).
9. **Nil inputs never panic**: a nil `Lookup`, nil env func, nil `*flag.FlagSet`, nil map find nothing; a nil
   `io.Reader` or `fs.FS` is `ErrJSON`; a nil `Option` is ignored.

## Default secret words (shared with `logging`)

`password, passwd, passphrase, secret, token, key, credential, private, dsn, authorization, cookie` (lead
decision, wave 6 review): matched as substrings of the normalised key (lower case; '-', '_', '.' and ' '
dropped, as `logging` normalises), so `Authorization`, `session_cookie` and `db.pass_word` are secret
(`config/values.go`, `defaultSecretWords`, a function, not a variable; test `TestValues_SharedDefaultWords`).
The list is identical in `config` and `logging`. A change to it is made in both packages in the same step.

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-CONFIG-001` | `Lookup` | `type Lookup func(key string) (string, bool)` |
| `TB-CONFIG-002` | `Layer` | `func Layer(ls ...Lookup) Lookup` |
| `TB-CONFIG-003` | `FromMap` | `func FromMap(m map[string]string) Lookup` |
| `TB-CONFIG-004` | `FromJSON` | `func FromJSON(r io.Reader) (Lookup, error)` |
| `TB-CONFIG-005` | `FromJSONFile` | `func FromJSONFile(fsys fs.FS, name string) (Lookup, error)` |
| `TB-CONFIG-006` | `FromEnv` | `func FromEnv(lookup func(string) (string, bool), prefix string) Lookup` |
| `TB-CONFIG-007` | `FromFlags` | `func FromFlags(fs *flag.FlagSet) Lookup` |
| `TB-CONFIG-008` | `String` | `func String(l Lookup, key, def string) string` |
| `TB-CONFIG-009` | `Int` | `func Int(l Lookup, key string, def int) (int, error)` |
| `TB-CONFIG-010` | `Duration` | `func Duration(l Lookup, key string, def time.Duration) (time.Duration, error)` |
| `TB-CONFIG-011` | `Bool` | `func Bool(l Lookup, key string, def bool) (bool, error)` |
| `TB-CONFIG-012` | `Required` | `func Required(l Lookup, keys ...string) error` |
| `TB-CONFIG-013` | `Values` | `type Values struct{ /* unexported; secrets never held */ }` + `String`, `GoString`, `Format`, `LogValue` |
| `TB-CONFIG-014` | `Collect` | `func Collect(l Lookup, keys []string, opts ...Option) Values` |
| `TB-CONFIG-015` | `Option` | `type Option func(*settings)` |
| `TB-CONFIG-016` | `WithSecrets` | `func WithSecrets(keys ...string) Option` |
| `TB-CONFIG-017` | `ErrMissing` | `var ErrMissing error` |
| `TB-CONFIG-018` | `ErrParse` | `var ErrParse error` |
| `TB-CONFIG-019` | `ErrJSON` | `var ErrJSON error` |
| `TB-CONFIG-020` | `EnvName` | `func EnvName(key string) string` |
| `TB-CONFIG-021` | `FlagName` | `func FlagName(key string) string` |

Options: `WithSecrets(keys...)`, default none beyond the eleven default words. Bounds: FromJSON 4 MiB, 64 levels.

Deviations from LADDER §4: §4 listed Lookup, Layer, FromMap, String, Int, Duration, Bool, Required. Added (by
the wave 6 brief): FromJSON, FromJSONFile, FromEnv, FromFlags, the redaction type (`Values` + `Collect` +
`Option` + `WithSecrets`; shape was the builder's call), and `EnvName`/`FlagName` (the key mapping made callable).
The brief's `Layer` question "later wins? first wins?" is answered: later wins (decision 1).

## Per export (shared facts)

- **Goroutines**: none, anywhere. **Blocking**: none, except that a `Lookup` calls the caller's own function,
  and `FromJSON` reads `r` (blocks as `r` does). **Cancellation**: no ctx (nothing waits; LADDER §4 shape).
- **Concurrency**: every returned `Lookup` is immutable (FromMap/FromJSON copy) or only reads its source
  (FromEnv: the caller's func; FromFlags: the FlagSet, not during Parse). `Values` is immutable.
- **Errors**: as in decisions 4-6; never a value in a message; never a panic on input; no logging; no exit.
- **Zero values**: nil `Lookup` finds nothing; zero `Values` prints "".
- **Compared against**: none. viper is a framework (global instance, reflection, file watching, remote
  stores); caarlos0/env and envconfig are reflection-based struct binders. Their shapes do not meet ours, so
  no compare/config/ (LADDER rank 31 names them as what this replaces, not as a behavioural twin).
- **Worked examples**: `ExampleLayer`, `ExampleLookup`, `ExampleFromMap`, `ExampleFromJSON`,
  `ExampleFromJSONFile`, `ExampleFromEnv`, `ExampleEnvName`, `ExampleFromFlags`, `ExampleFlagName`,
  `ExampleString`, `ExampleInt`, `ExampleDuration`, `ExampleBool`, `ExampleRequired`, `ExampleCollect`,
  `ExampleValues` (slog), `ExampleWithSecrets`; all with `// Output:`; no Example output contains a secret
  value given to it.

## Tests (written; not run by the builder)

No process environment (env is `envOf(map)`), no files (`fstest.MapFS`, `strings.Reader`), fresh
`flag.FlagSet` per test, no clock, no goroutines; `TestMain` uses `leak.Main`. Deterministic (map iteration only
over independent cases).
- Precedence across all four layers (`TestLayer_Precedence`), empty-overrides, nil layers, argument copy.
- FromEnv mapping table, set-but-empty, nil func; FromFlags only-set, read-at-lookup with bool/duration flags, nil.
- FromJSON flattening (numbers literal, big ints, null, arrays, dotted names, case-sensitivity), 20 ErrJSON
  cases (syntax, truncation, non-object, trailing, duplicates, collisions, empty key, bad arrays, depth 65,
  > 4 MiB in a value and in trailing space), depth 64 accepted, syntax error offset only; FromJSONFile missing
  (fs.ErrNotExist), invalid path, malformed (names the file), nil fs.
- Every typed getter's parse error (syntax, range, empty, bare duration, "yes"), defaults on unset, nil Lookup.
- Required: every missing key listed, order, duplicates once, `Unwrap() []error` parts each wrap ErrMissing.
- Failing assertions never print the secret either: every message that could hold it goes through `masked`
  (replaces the secret with `<SECRET>`) or prints a length or an index instead (review SHOULD FIX).
- `TestFromJSON_SyntaxErrorOffsetOnly`: a syntax-error message holds no input character.
- Redaction: `TestValues_NoSecretInAnyOutput` searches String, GoString, %v, %+v, %#v, %s, %q, %x, %d, pointer
  forms, slice/struct embedding, Println, slog text and JSON; `TestDefect_go72_SecretLogged` searches every
  getter error, Required, six malformed-JSON errors containing the secret, and a slog line.
- Defects: the six `TestDefect_go17_* / go31_* / go72_*` above.

## Out of scope

YAML/TOML files (an adapter, `adapters/yaml`, LADDER §3); struct binding by tags (reflection: rejected by
LADDER §4); file watching / hot reload; remote stores; secret managers (an adapter implementing `Lookup`);
value-pattern redaction (a secret inside a non-secret key's value: the `logging` package's value patterns);
list getters (arrays arrive as comma-joined strings; a `Strings` getter can be added later with a new ID).

## For the lead

- INDEX rows TB-CONFIG-001..021 (doc.go lists them); taxonomy `config` status planned → built.
- DECISIONS candidates: decisions 1 (later wins), 2 (empty is set; Required counts empty as missing), 6
  (duplicate keys are errors; arrays comma-joined; null unset), 7 (redaction drops the secret at Collect; the
  eleven default words, shared with logging). ASSUMPTIONS: the 4 MiB / 64-level JSON bounds; `EnvName`/`FlagName` exported (two IDs
  the brief did not list).
- doc/sources: 12factor.net/config (TB-CONFIG-001..003, 006, 012..014, 016); pkg.go.dev os, flag,
  encoding/json, io/fs, strconv, time, errors, fmt, log/slog.
- No compare/ (viper is a framework). No calcbench requests.

REMAINING: none. Review fixes (2026-11-13): secret masked in failing-assertion messages
(config_test.go `noSecret`/`masked`, defects_test.go, values_test.go); JSON syntax errors by offset only
(json.go `wrap`); default secret words made a function and stated above as the shared list.
