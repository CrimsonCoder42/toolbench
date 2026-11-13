# Brief for the wave 4 builders (T2, 2026-11-13)

The standard, the mechanics and the hard limits are those of `doc/wave3_2026-11-13/BRIEF.md` (read it first: the
standard verbatim, owned paths, doc.go and doc-comment shape, test rules, no `go mod tidy` by builders, hand-back
format). Request: `doc/requests/waves3_4_2026-11-13.md`. Folder for this wave: `doc/wave4_2026-11-13/`. Model specs:
`doc/specs/pool.md`, `doc/specs/batch.md` (timed, uses clock.Fake), `doc/specs/flow.md`.

## Built on (import these; never duplicate them)

- `clock`: `Clock` (Now, NewTimer, AfterFunc, NewTicker), `Real()`, `Fake` (`Advance`, `AdvanceContext`,
  `BlockUntil(ctx, n)`). Every timed export takes `WithClock(clock.Clock)` (default `clock.Real()`); every timed test
  uses `clock.Fake` or `testing/synctest`. Read `doc/specs/clock.md` (D-009, D-010, D-012).
- `panics`: `Try`/`TryValue`/`ErrPanic` for every recover of caller code on our goroutines.
- `group` (errgroup shape, first error cancels, Wait), `flow`, `semaphore` where they fit.
- Lessons from waves 1-3: a plain `New` returning `*T` needs its own ID; exported `Err...` used by callers get an
  ID; nil ctx = `context.Background()`; invalid counts and durations are an error or a documented default, never a
  panic or an unbounded block; tests deterministic under `-count=200`; a request already received is always run
  (wave 2 guard blocker); a value already taken from an input is never silently passed over (wave 2 flow blocker).

## Wave 4 packages and the agreed LADDER.md §4 APIs

**B5 `schedule`, SHORT SCHED** (rank 20). Repos go_49_task_scheduling, go_50_task_scheduling_part_2,
go_37_background_work (checkin/ heartbeat) (defects: `for {}` busy spin; signal goroutine + os.Exit; goroutine on
`time.After(time.Until(t))` with no ctx; Sleep loops). Authority: pkg.go.dev `time` (`Timer`, `Ticker`, go1.23
semantics); Cox-Buday, Concurrency in Go, 2017, ch.5 "Heartbeats" (not online: cite, say not opened); IEEE Std
1003.1-2024 "crontab" (for the PENDING Cron only). calcbench: NUM-PROB-059 UniformQuantile for `WithJitter` (u from
math/rand/v2 with an injectable source, fixed seed in tests); ALG-HEAP-002/004/005 only if Run ever orders several
next-run times (one schedule per Run: probably not; say so). Builds on: `clock`, `panics`. Compare: none
(gocron/robfig are named in LADDER but are cron libraries; Cron is not built).
**Ship now**: Every, At, Run, Heartbeat. **Cron**: write `ParseCron` in the spec (signature, calcbench ID marked
`PENDING calcbench R-3`, doc/wave1_2026-11-13/calcbench_requests.md) and do NOT build it.
```go
type Schedule interface{ Next(after time.Time) time.Time } // zero Time = no further runs
func Every(d time.Duration) Schedule                         // d <= 0: decide (never fires? error-free contract)
func At(t time.Time) Schedule                                // one shot (LADDER.md §4 has no At: request adds it; record the deviation)
func Run(ctx context.Context, s Schedule, task func(context.Context) error, opts ...Option) error
func Heartbeat(ctx context.Context, interval time.Duration, opts ...Option) (beat func(), pulses <-chan time.Time)
type Option func(*config)
func WithClock(c clock.Clock) Option
func WithJitter(frac float64) Option     // ± fraction of the interval, via NUM-PROB-059
func WithOnError(f func(error)) Option   // default: Run returns the first task error? or continues? decide and state
// PENDING (spec only): func ParseCron(spec string, loc *time.Location) (Schedule, error)
```
State: no overlap (one run at a time; a run longer than the period: skip missed or run late? decide), what Run
returns (ctx cause when cancelled; task errors per WithOnError), a panicking task (ErrPanic via OnError or return),
Heartbeat's pulse channel (who closes it, drops when nobody reads — a heartbeat must never block the worker: the
one named drop in toolbench, documented per rule 5).

**B6 `lifecycle`, SHORT LIFE** (rank 19). Repos go_42_faktory (ShutdownTimeout), go_37_background_work (SIGTERM then
pool.Stop), go_80_tcp_udp_sockets (no shutdown path; connection goroutines leak). Authority: pkg.go.dev net/http
`Server.Shutdown` (the drain-with-deadline model); pkg.go.dev os/signal `NotifyContext` (go1.26: the cause names
the signal); The Twelve-Factor App, "IX. Disposability" (https://12factor.net/disposability). Builds on: `group`,
`clock` (the drain deadline), `panics`. Compare: github.com/oklog/run (`Group.Add(execute, interrupt)`, `Run`) —
write compare/lifecycle/; the lead tidies. The request: "run group with ordered start, reverse-order stop, drain
deadline, signal hook as an option (no os/signal in core tests)". §4 had
`Run(ctx, grace, start func(context.Context) error, stops ...Stopper) error`; extend it into a run group with ordered
components, and record the deviation:
```go
type Stopper func(ctx context.Context) error             // §4
func Run(ctx context.Context, grace time.Duration, start func(context.Context) error, stops ...Stopper) error // §4
// plus, for ordered start / reverse stop (shape is the builder's call; keep it small and familiar), e.g.:
type Component struct{ Name string; Start func(context.Context) error; Stop Stopper }
func (g *Group) Add(c Component); func (g *Group) Run(ctx context.Context, opts ...Option) error
func WithGrace(d time.Duration) Option; func WithClock(c clock.Clock) Option
func WithNotify(notify func(context.Context) (context.Context, context.CancelFunc)) Option // the signal hook: the
//   caller passes e.g. func(ctx){ return signal.NotifyContext(ctx, os.Interrupt) }; core never imports os/signal
```
State: start order and what happens when component k fails to start (stop k-1..1 in reverse), a component whose
Start returns early (nil vs error), the grace deadline (stops get a ctx that expires at grace; after it, Run returns
an error naming who did not stop), errors.Join of stop errors, a panic in Start/Stop (ErrPanic), Run called twice.

**B7 `pubsub`, SHORT PUBSUB** (rank 22). Repos socket_io_emitter, go_44_kafka, go_58_rabbitmq (defects: producer
opened per request; consumer goroutine never exits; one channel shared across goroutines; auto-ack loses messages;
spin on a closed channel's zero value). Authority: Hohpe & Woolf, EIP, "Publish-Subscribe Channel"
(https://www.enterpriseintegrationpatterns.com/patterns/messaging/PublishSubscribeChannel.html), "Event-Driven
Consumer", "Point-to-Point Channel". calcbench: ALG-QUEUE-002 CircularBuffer for `WithDropOldest` (under the
subscriber's mutex; calcbench queues are not concurrent-safe). Builds on: `panics` (a handler panic), `flow` or
`group` where they fit. Compare: github.com/ThreeDotsLabs/watermill gochannel only if it fits cleanly without a heavy
dependency tree; otherwise none (say why). The interfaces are what T5 packages and the adapters will implement:
keep them minimal and stable.
```go
type Publisher[M any] interface{ Publish(ctx context.Context, topic string, msgs ...M) error }
type Subscriber[M any] interface{ Subscribe(ctx context.Context, topic string, h func(context.Context, M) error) error }
// Subscribe blocks until ctx is done (or the bus closes) and the handler has returned; returns the cause.
func NewBus[M any](opts ...Option) *Bus[M]       // implements both; zero Bus usable? decide
func (b *Bus[M]) Close() error
type Option func(*config)
func WithBuffer(n int) Option                     // per subscriber; default 0? decide (backpressure: Publish blocks)
func WithDropOldest(report func(topic string, m M)) Option // the explicit, named drop (rule 5)
```
State: delivery is AT-MOST-ONCE in process (documented); Publish blocks on a slow subscriber unless WithDropOldest
(backpressure, rule 5); ordering per subscriber per topic; Publish with no subscribers; Publish after Close; a
handler error (logged nowhere: returned? ignored? → decide; the T5 queue adds retry/dead-letter); handler panic →
ErrPanic (Subscribe returns it? keeps running?); Subscribe on a closed bus; many topics; unsubscribe = cancel ctx.
