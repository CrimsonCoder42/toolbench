# pubsub: in-process publish-subscribe (Bus) and the Publisher / Subscriber interfaces

Status: SHIPPED (2026-11-13)
Package: `pubsub` · SHORT: `PUBSUB` · Layer: core · Tier: T2
Work folder: `doc/wave4_2026-11-13/` · Ladder rank: `doc/ladder_2026-11-13/LADDER.md` #22
Builds on: `panics` (TB-PANIC-001 ErrPanic, TB-PANIC-003 Try), calcbench `algo/queue` (ALG-QUEUE-002) ·
`flow`/`group`: not used (below) · Decisions: none new · Assumptions: see "For the lead"

## Research

### a. The reference (code-heim; reading list only, never cited)

| Repo | What it does | Outdated / defects (become tests) |
|---|---|---|
| `_reference/code-heim/go_44_kafka/producer/main.go` | HTTP handler `placeOrder` → `PushOrderToQueue` → `ConnectProducer` (sarama SyncProducer). | **Producer opened per request** (`:31-39`: connect, `defer producer.Close()` inside every call). `r.Method != POST` check (`:63`; go1.22 method patterns). Ours: one Bus shared by every caller → `TestPublish_defectGo44Go58_oneBusSharedByConcurrentPublishers`. |
| `_reference/code-heim/go_44_kafka/worker/main.go` | `ConsumePartition`, a goroutine selecting on Errors/Messages/signal (`:36-51`). | **Consumer goroutine never exits** (`:37-50`: after `doneCh <- struct{}{}` the `for` loops again; a second signal blocks forever on the unbuffered `doneCh`). `signal.Notify` by hand (`signal.NotifyContext`). `msgCnt` shared without a guard. → `TestSubscribe_defectGo44_consumerGoroutineExits` (5 subscribers, cancel, all return, leak check). |
| `_reference/code-heim/go_58_rabbitmq/producer/main.go` | gin route publishes on one `amqp.Channel` (`:52`). | **One AMQP channel shared across concurrent handler goroutines** (amqp091 channels are not safe for concurrent publish); `err` from the outer scope reassigned inside the handler (`:52`, a data race); `Publish` without ctx (`PublishWithContext` + confirms today). → same concurrent-publishers test under `-race`. |
| `_reference/code-heim/go_58_rabbitmq/consumer/main.go` | `Consume(..., autoAck=true, ...)` (`:30`), `for { select { case message := <-messages: ... } }` (`:38-45`). | **Spin on a closed channel's zero value** (`:40`: no `, ok`; when the broker closes `messages` the loop logs empty bodies forever) → `TestSubscribe_defectGo58_noSpinAfterClose`. **Auto-ack loses messages** silently (`:30`) → `TestSubscribe_defectGo58_failedMessageNotSilentlyAcked` and `TestBus_undeliveredCounted`. `os.Exit(0)` from the signal case skips deferred closes. |
| `_reference/code-heim/socket_io_emitter/emitter.go` | socket.io emitter over Redis (`:9-20`). | `github.com/go-redis/redis/v8` (now `github.com/redis/go-redis/v9`, LADDER.md §6); no error checked; library usage only. It is a publish to a broker topic ("room"): the shape a later `adapters/redis` Publisher implements. |

### b. The authority (opened 2026-11-13)

- Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, **"Publish-Subscribe Channel"**. Opened:
  https://www.enterpriseintegrationpatterns.com/patterns/messaging/PublishSubscribeChannel.html, 2026-11-13.
  Relied on: problem "How can the sender broadcast an event to all interested receivers?"; solution "Send the
  event on a Publish-Subscribe Channel, which delivers a copy of a particular event to each receiver"; "When an
  event is published into the channel, the Publish-Subscribe Channel delivers a copy of the message to each of
  the output channels"; "Each subscriber only gets the message once and consumed copies disappear from their
  channels". Ours: one buffer (output channel) per Subscribe; Publish puts a copy in each; at most once each.
- Hohpe & Woolf, EIP, **"Event-Driven Consumer"**. Opened:
  https://www.enterpriseintegrationpatterns.com/patterns/messaging/EventDrivenConsumer.html, 2026-11-13. Relied
  on: "An application needs to consume Messages as soon as they're delivered"; "one that is automatically handed
  messages as they're delivered on the channel"; "invoked by the messaging system when a message arrives on the
  consumer's channel". Ours: Subscribe takes a handler (callback), not a receive channel; the Bus calls it.
- Hohpe & Woolf, EIP, **"Point-to-Point Channel"**. Opened:
  https://www.enterpriseintegrationpatterns.com/patterns/messaging/PointToPointChannel.html, 2026-11-13. Relied
  on: "ensures that only one receiver consumes any given message"; "If the channel has multiple receivers, only
  one of them can successfully consume a particular message". Used to draw the boundary: the Bus is NOT
  point-to-point; two Subscribe calls on one topic each get every message. Competing consumers on one queue are
  `pool` (T2) and `jobs` (T5).
- Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Stopping short" (the
  buffered-channel section: WithBuffer's bound) and "Explicit cancellation"
  (opened for flow.md, same day): a blocked send always also selects on done; WithBuffer's bound.
- pkg.go.dev `net/http` "ErrServerClosed" (ErrClosed); `errors` "Is" (ErrInvalid); Gamma et al., Design
  Patterns, 1994, "Observer" (WithOnSubscribers; book, not opened online); Pike 2014 functional options (Option).

### c. Today's Go (GO_RELEASES.md)

- The standard library has no topic-based publish-subscribe. A channel is point-to-point (one receiver takes
  each value); broadcast needs a copy per receiver, a subscriber registry, unsubscribe, and an order.
- `context.WithCancelCause` / `context.Cause` (1.20): a handler's ctx is cancelled with cause ErrClosed by
  Close. `sync.WaitGroup.Go` (1.25) and `testing/synctest` (1.25) in tests: "Publish is blocked" is a synctest
  fact, never a timing. `slices.DeleteFunc`/`Clip` for the copy-on-write subscriber list.
- Why toolbench still needs it: the repos hand-roll consumers that never exit, spin on closed channels and drop
  messages silently; T5 `jobs` and `outbox` and the broker adapters need ONE small interface pair to target.

### d. calcbench

- **ALG-QUEUE-002 CircularBuffer** (confirmed in calcbench INDEX.md:199), policy `Overwrite`, capacity
  `max(WithBuffer n, 1)`, one per subscriber, used only with WithDropOldest, always under the subscriber's
  mutex (calcbench queues are not concurrent-safe). Push reports only *that* it overwrote, so the dropped value
  is read with `Front()` just before `Push` on a full ring (pubsub/subscription.go, `push`; `Push`'s overwritten result decides the report).
- Not used for the backpressure path: there the buffer is a Go channel, because Publish must block on it with a
  ctx and Close case (LADDER.md §5 "re-implemented on purpose"). Nothing in calcbench is re-implemented.

### e. Built on

- `panics.Try` wraps every handler call, every WithDropOldest report and every WithOnSubscribers hook.
- `flow` and `group` are **not used**: the Bus starts no goroutine (Subscribe runs on the caller's goroutine,
  Publish on the publisher's), so there is nothing to group; flow's channel stages would add a goroutine per
  subscriber and lose the per-subscriber buffer bound and the ordered fan-out. A caller wanting N subscribers
  as one unit runs them in a `group.Group`.
- Options are **type-safe**: `type Option[M any] func(*config[M])`, so a WithDropOldest report or any option for
  another message type does not compile (reviewer finding, D-026). The cost is an explicit type argument on
  `WithBuffer[M](n)` and `WithOnSubscribers[M](f)` (Go does not infer M from the result); `WithDropOldest`
  infers M from report.

## Exports

| ID | Name | Signature |
|---|---|---|
| `TB-PUBSUB-001` | `Publisher` | `type Publisher[M any] interface{ Publish(ctx context.Context, topic string, msgs ...M) error }` |
| `TB-PUBSUB-002` | `Subscriber` | `type Subscriber[M any] interface{ Subscribe(ctx context.Context, topic string, h func(context.Context, M) error) error }` |
| `TB-PUBSUB-003` | `Bus` | `type Bus[M any] struct{...}`; methods `Publish`, `Subscribe` (as above), `Close() error` |
| `TB-PUBSUB-004` | `NewBus` | `func NewBus[M any](opts ...Option[M]) *Bus[M]` |
| `TB-PUBSUB-005` | `Option` | `type Option[M any] func(*config[M])` |
| `TB-PUBSUB-006` | `WithBuffer` | `func WithBuffer[M any](n int) Option[M]` |
| `TB-PUBSUB-007` | `WithDropOldest` | `func WithDropOldest[M any](report func(topic string, m M)) Option[M]` |
| `TB-PUBSUB-008` | `WithOnSubscribers` | `func WithOnSubscribers[M any](f func(topic string, subscribers int)) Option[M]` |
| `TB-PUBSUB-009` | `ErrClosed` | `var ErrClosed = errors.New("pubsub: bus closed")` |
| `TB-PUBSUB-010` | `ErrInvalid` | `var ErrInvalid = errors.New("pubsub: invalid argument")` |

Code: pubsub/publisher.go:15, subscriber.go:17, bus.go:33 (Publish :95, Subscribe :150, Close :238),
bus.go:58, options.go:14, :33, :49, :66, errors.go:12, :23. Internals: subscription.go.

| Option | Default | Bound it sets |
|---|---|---|
| `WithBuffer(n)` | 0 (direct hand-off); n < 0 is 0; n > 2^20 makes the Bus invalid (ErrInvalid) | messages waiting per subscriber |
| `WithDropOldest(report)` | off (backpressure) | ring of max(n,1) per subscriber; overflow drops oldest, reported |
| `WithOnSubscribers(f)` | none | — (observability hook) |

**Deviations from LADDER.md §4 / BRIEF B7**: (1) `Option[M]` is generic (D-026): options are checked by the
compiler against the Bus's message type, so there is no runtime type-mismatch error; `WithBuffer[M]` and
`WithOnSubscribers[M]` take an explicit type argument.
(2) Added `WithOnSubscribers`: the Bus starts no goroutine and Subscribe blocks, so a caller (and every Example)
needs a non-polling way to know a subscriber is registered before publishing; it doubles as the metrics hook.
(3) Added `ErrClosed`, `ErrInvalid`. (4) The maximum buffer is an unexported constant (2^20), named in docs.

## Decisions taken (BRIEF B7 points)

- **Delivery: AT-MOST-ONCE in process.** Each subscriber registered when a message is published gets it at most
  once; a subscriber registered later does not get it (no replay); a message in a subscriber's buffer when it
  leaves is lost, but **counted**: Subscribe's error becomes `pubsub: N buffered messages not delivered: <reason>`
  (wraps the reason). Retry, redelivery and dead letters are T5 `jobs`/`outbox`.
- **Backpressure**: Publish blocks while a subscriber of the topic is busy and its buffer is full (default
  buffer 0). Fan-out is sequential per message, so one slow subscriber holds the publisher (and with it later
  messages to the others): stated, tested. **WithDropOldest** is the one named drop: never blocks, reports
  every dropped message on the publishing goroutine, outside locks.
- **Ordering**: per topic, Publish calls take turns (a capacity-1 turn channel; waiting for it selects on ctx and
  Close), so every subscriber of a topic sees the same order; one call's msgs are consecutive and in order;
  topics are independent.
- **Publish with no subscribers**: nil, delivered to nobody (EIP: a copy per receiver; zero receivers, zero
  copies). Topics exist only while they have subscribers (no growth with dead topics).
- **Publish after Close / Subscribe on a closed bus**: ErrClosed. A Publish blocked on a subscriber or waiting
  for its turn returns ErrClosed when Close is called. Close returns nil, idempotent, does not wait for
  subscribers (they run on their callers' goroutines; Subscribe's return is the wait point); callable from a
  handler.
- **Handler error**: ends that subscription; Subscribe returns it (not logged, not swallowed: rule 7). Other
  subscribers are untouched. A caller that wants keep-going wraps h to return nil; T5 adds retry/dead-letter.
- **Handler panic**: same path, `*panics.PanicError` (ErrPanic); the Bus keeps working.
- **Subscribe** never returns nil; it returns after the handler has returned; the handler's ctx is cancelled by
  the caller's ctx or by Close (cause ErrClosed). A done ctx wins over a ready message (no handler call starts
  after cancel is seen). Unsubscribe = cancel ctx. A subscriber leaving while Publish waits on it is skipped.
- **Zero value**: ready to use (no buffer, backpressure, no hook). nil `*Bus`: Publish/Subscribe ErrClosed,
  Close nil. nil ctx = Background. nil handler = ErrInvalid (not ErrPanic on the first message, as pool's nil
  fn: here nothing would ever surface it until a message arrives).

## Per export

### `TB-PUBSUB-001` `Publisher`, `TB-PUBSUB-002` `Subscriber`
- **Pattern**: publish-subscribe channel / event-driven consumer. **Authority**: EIP 2003 (above).
- **Familiar shape**: watermill's `message.Publisher.Publish(topic, msgs...)` and `Subscriber.Subscribe(ctx,
  topic)`, with ctx first, generics instead of `[]byte` payloads, and a handler instead of a returned channel
  (Event-Driven Consumer; no channel to forget to drain). One method each, so adapters and T5 implement them.
- **Contract** for every implementation: Publish safe for concurrent use and ctx-bounded; Subscribe blocks, never
  returns nil, returns after h has returned, unsubscribe is cancel; the fate of a message whose h failed is the
  implementation's documented choice (Bus: lost).
- Examples: `ExamplePublisher`, `ExampleSubscriber`.

### `TB-PUBSUB-003` `Bus` (Publish, Subscribe, Close), `TB-PUBSUB-004` `NewBus`
- **Goroutines**: none started. **Cancellation**: Publish: ctx done before → its cause; while waiting (turn or
  subscriber) → its cause, messages already handed over stay delivered. Subscribe: before → cause, nothing
  registered; mid → after the current handler call; after a handler error → that error.
- **Errors**: ErrClosed, ErrInvalid (nil h, buffer > 2^20), ctx cause, handler error, ErrPanic
  (handler, report, hook), wrapped with the undelivered count when non-zero.
- **Blocking**: see Decisions. Handler publishing to its own unbuffered topic waits for itself (documented).
- **Concurrency guard**: `Bus.mu` guards topics/closed/done; the subscriber list is copy-on-write and read as a
  snapshot. `subscriber.mu` (RWMutex): a backpressure Publish holds it for reading while it may send, a
  drop-oldest Publish holds it to push; Subscribe's exit cancels the subscriber ctx first (waking blocked
  publishers), then takes it to mark closed and count the buffer, so no message lands uncounted.
- **Zero value**: ready to use; not copied after first use.
- **Compared against**: none (below). Examples: `ExampleBus`, `ExampleNewBus`, `ExampleBus_Publish`,
  `ExampleBus_Subscribe`, `ExampleBus_Subscribe_panic`, `ExampleBus_Close`.

### `TB-PUBSUB-005..008` options, `TB-PUBSUB-009..010` sentinels
- Table above. `WithDropOldest`: Math ALG-QUEUE-002. Examples: `ExampleOption`, `ExampleWithBuffer` (`<nil>
  [1 2 3]`), `ExampleWithDropOldest` (`dropped [2 3] got [1 4]`), `ExampleWithOnSubscribers`,
  `ExampleErrClosed`, `ExampleErrInvalid`.

## Tests (`pubsub/bus_test.go`, `pubsub/example_test.go`)

`TestMain` with `leak.Main`; `leak.Verify(t)` in every test; bounded by `context.WithTimeout(t.Context(), 10s)` or
a synctest bubble; no Sleep, no polling ("is registered" via the WithOnSubscribers hook or `synctest.Wait`).
Fan-out to every subscriber (direct, buffered, drop-oldest with room) · same order for every subscriber with 4
concurrent publishers and per-publisher order · backpressure: Publish blocks on a slow subscriber (synctest) ·
a busy subscriber holds Publish for the others · WithDropOldest reports exactly what it dropped (buffer 0/1/2/3;
delivered + reported = published) · report panic → ErrPanic · no subscribers · closed / Close twice / zero / nil
*Bus · invalid options, nil handler, valid edges · nil ctx · handler error ends only that subscriber · handler
panic → ErrPanic, bus keeps working · hook panic · Subscribe returns on cancel and on Close only after the
handler, handler ctx cause (Canceled / ErrClosed) · cancel before start · undelivered counted (both modes) ·
Publish cancelled while blocked and while waiting for its turn; leaving subscriber skipped · Close unblocks
Publish · 20 topics, no crossing · defects go_44 ×2, go_58 ×2 · `BenchmarkBus_Publish` (ReportAllocs). Each
is deterministic under `-count=200`.

## Compare

**None.** watermill's `pubsub/gochannel` lives in the root module `github.com/ThreeDotsLabs/watermill`, which pulls
`github.com/google/uuid`, `github.com/lithammer/shortuuid/v3`, `github.com/oklog/ulid`, `github.com/pkg/errors`,
`github.com/sony/gobreaker`, `github.com/cenkalti/backoff/v4` and more into compare/go.mod for one test; and the
shapes do not meet: gochannel carries `*message.Message` with `[]byte` payloads and UUIDs, needs Ack/Nack, and
returns a channel from Subscribe (not a handler). A comparison would test adapters around both, not the
behaviour. Revisit when the T5 `outbox` compares against watermill's forwarder.

## Out of scope

Persistence, replay and durable subscriptions (adapters); retry, redelivery, dead letters, acks (T5 `jobs`,
`outbox`); point-to-point / competing consumers (`pool`, `jobs`); wildcard or hierarchical topics; per-topic
options; a goroutine per subscriber with an async handler (the caller runs Subscribe where it wants);
metrics beyond the subscriber-count hook (a later additive option).

## For the lead

- INDEX.md rows TB-PUBSUB-001..010 (doc.go lists them). doc/taxonomy row: `pubsub` deps are panics + calcbench
  algo/queue (LADDER.md §3 listed flow: not used, see e.).
- doc/sources: EIP "Publish-Subscribe Channel" (001, 003, 004, 007), "Event-Driven Consumer" (002, 003),
  "Point-to-Point Channel" (spec boundary only); go.dev/blog/pipelines "Stopping short" (006); net/http
  "ErrServerClosed" (009); errors "Is" (010); Gamma et al. "Observer" (008, book not opened); Pike 2014 (005).
- ASSUMPTIONS: (1) handler error ends the subscription and is returned (not keep-going); (2) Close does not wait
  for subscribers; (3) WithBuffer cap 2^20; (4) nil handler is ErrInvalid; (5) Publish takes a per-topic turn
  (one order for all subscribers) at the cost of serialising publishers of one topic; (6) WithDropOldest with no
  buffer keeps 1; (7) the added WithOnSubscribers export.
- D-026: `Option[M]` (compile-time type-safe options) WON over a non-generic `Option` with a runtime check of
  the WithDropOldest report's type (lost: a wrong type compiled and surfaced only as ErrInvalid at the first
  Publish/Subscribe). No ID removed: TB-PUBSUB-001..010 unchanged; ErrInvalid (010) stays for a nil handler
  and WithBuffer above 2^20. INDEX row 006 Authority is now "Stopping short" (regenerate).
- calcbench requests: none. compare/pubsub: not created.
- Checks: `gofmt -l pubsub` empty, `go build ./...` ok, `go vet ./pubsub/` clean;
  `go test -race -timeout 5m -count=50 ./pubsub/` ok; `./testing/` fails only on INDEX.md:79 (TB-PUBSUB-006
  Authority, stale until the lead regenerates INDEX.md).

REMAINING: none.

## Align (2026-11-13)

Work folder `doc/align_2026-11-13/` · task D5 (decided by Devin) · additive only · TB-PUBSUB-001..010 unchanged.

### Matrix rows

| Row | Pattern | REPEAT | Verdict |
|---|---|---|---|
| 26 | in-process publish-subscribe bus | 0/6/6 | SAME; the policy and envelope gaps are rows 36, 37 |
| 36 | slow-subscriber policy (evict) | 0/2/2 | PARTIAL: enters by the scale clause (sequential fan-out behind a full chan, bus.go:124-130) |
| 37 | message envelope (headers, correlation id) | 0/2/2 | MISSING: enters by authority (EIP "Message", "Message Header", "Correlation Identifier") |
| 69 | delivery / observer hook | 0/1/1 | NEAR: built only because D5 names it |
| (b) rank 6 | envelope · block / drop-oldest / drop-subscriber · OnDelivered | — | D5 |
| §6 | one `Bus.mu` for every topic on every Publish; sequential fan-out behind a full chan | — | drop-subscriber policy + `BenchmarkBus_Publish_manyTopicsManyPublishers` (measured in VERIFY; sharding only if the number says so) |

### References read (reading list, never authority)

- `_reference/AkhilSharma90/LLM-Projects/hard-resume-agent/hub.go:43-48` and
  `_reference/AkhilSharma90/Videocall-Chat-GO-Project/pkg/chat/hub.go:31-36`: `select { case client.send <- m:
  default: close(send); delete(client) }` on the hub goroutine: a full buffer evicts the client, the publisher
  never blocks. Defect: the hub closes `send` while the client goroutine may still be sending on it (the
  close-from-the-hub race); `run` has no ctx and never returns.
- `Kafka-GO-Real-Time-Feed/producer/main.go:93-94` (watermill `middleware.SetCorrelationID` on message metadata),
  `consumer/main.go:68-70` (`middleware.CorrelationID` copies the consumed message's id onto every produced
  message); `GO-CQRS/otel/utils.go:38-47` (AMQP header carrier: inject/extract trace context through a
  `map[string]string`-shaped header set; file not present in `_reference` today, line numbers from the matrix).
- `GO-Fafka/main.go:90-95` (watermill Prometheus router metrics: per-handler latency and counts, as middleware).

### Authority (opened 2026-11-13; `.claude/rules/sources.md`)

- Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, **"Message"**. Opened:
  https://www.enterpriseintegrationpatterns.com/patterns/messaging/Message.html. Relied on: "Package the
  information into a Message, a data record that the messaging system can transmit through a message channel";
  header: "Information used by the messaging system that describes the data being transmitted, its origin, its
  destination, and so on"; body: "The data being transmitted; generally ignored by the messaging system and
  simply transmitted as-is". Ours: `Message[T]{Headers, Body}`; the Bus never reads Headers (Body is "as-is").
  "Message Header" is the header section of this page (no separate page).
- Hohpe & Woolf, EIP, 2003, **"Correlation Identifier"**. Opened:
  https://www.enterpriseintegrationpatterns.com/patterns/messaging/CorrelationIdentifier.html. Relied on: "Each
  reply message should contain a Correlation Identifier, a unique identifier that indicates which request
  message this reply is for"; the replier "gets the request ID from the request and stores it as the
  correlation ID in the reply". Ours: `HeaderRequestID`, `HeaderCorrelationID`; a handler copies the inbound
  id into the message it republishes (ExampleMessage).
- Hohpe & Woolf, EIP, 2003, **"Publish-Subscribe Channel"** (opened for the base, same day): the delivery policy
  of a full output channel is the channel's; WithDropSubscriber is the third policy the Bus (the channel) offers.
- **WithDropSubscriber has no authority describing eviction itself.** EIP "Publish-Subscribe Channel" only
  places the delivery policy with the channel (so the Bus may offer it); Ajmani 2014 "Stopping short" is about
  receivers stopping early, not about a channel evicting a slow receiver, and is NOT cited for it. The eviction
  shape comes from the reading list only: the Gorilla WebSocket chat hub as copied by the two reference hubs
  above. The doc comment carries the one EIP line with that qualification (reviewer finding, 2026-11-13).
- Gamma, Helm, Johnson, Vlissides, Design Patterns, Addison-Wesley, 1994, "Observer": book, **not opened**;
  WithOnDelivered is the same observer shape as WithOnSubscribers (TB-PUBSUB-008).
- pkg.go.dev `time`, "func Now" and "func (Time) Sub" (WithClock measures with Now().Sub, as breaker's WithClock).

### Today's Go (go1.27.3)

`context.WithCancelCause` / `context.Cause` carry `ErrSlowSubscriber` to the evicted handler and to Subscribe's
result; `maps.Clone` for WithHeader's copy-on-write; `testing/synctest` for "Publish did not block"; `b.Loop` and
`b.RunParallel` in the benchmarks. Nothing new in go1.27 changes the shape.

### calcbench

None new. ALG-QUEUE-002 stays with WithDropOldest only; WithDropSubscriber uses a Go channel (a non-blocking
send with `default` is the policy, and the existing backpressure path already owns the channel).

### New exports (all additive; `doc.go` lists them)

| ID | Name | Signature | Contract |
|---|---|---|---|
| `TB-PUBSUB-011` | `Message` | `type Message[T any] struct { Headers map[string]string; Body T }`; methods `Header(key) (string, bool)`, `WithHeader(key, value) Message[T]` | pure value; zero value ready (nil Headers = no headers; Header on it is `"", false`; WithHeader on it allocates a one-entry map). WithHeader is copy-on-write: a new Message with a cloned map, the receiver unchanged. Delivered by value, the map SHARED read-only among a topic's subscribers: a handler must not mutate `m.Headers`; it calls WithHeader. No goroutines, never blocks, no errors. Methods carry no ID (as Guarded.Do). |
| `TB-PUBSUB-012` | `HeaderRequestID` | `const HeaderRequestID = "request-id"` | the header name adapters map to their broker's request-id header |
| `TB-PUBSUB-013` | `HeaderCorrelationID` | `const HeaderCorrelationID = "correlation-id"` | the header name adapters map to their broker's correlation-id header; a reply/republish copies the inbound id here |
| `TB-PUBSUB-014` | `ErrSlowSubscriber` | `var ErrSlowSubscriber = errors.New("pubsub: slow subscriber")` | the cause of an evicted subscriber's ctx and of its Subscribe result (wrapped with the undelivered count by the existing close path, so `errors.Is` matches) |
| `TB-PUBSUB-015` | `WithDropSubscriber` | `func WithDropSubscriber[M any](report func(topic string, m M)) Option[M]` | third slow-subscriber policy. Full buffer at Publish time: that subscriber's ctx is cancelled with cause ErrSlowSubscriber, the undeliverable message goes to report(topic, m) on the publishing goroutine outside every lock, Publish continues to the other subscribers and does not block. nil report = evict without report (explicit, like WithDropOldest). Buffer is max(WithBuffer n, 1): with no buffer "full" would mean "no handler waiting", which would evict every subscriber whose handler is between two messages, so the drop policies both keep one slot. An evicted subscriber gets no further message (skipped, like one that left). A panic in report is Publish's error (ErrPanic); the eviction has already happened. No goroutines. |
| `TB-PUBSUB-016` | `WithOnDelivered` | `func WithOnDelivered[M any](f func(topic string, m M, d time.Duration, err error)) Option[M]` | called once after each handler call returns, on the subscriber's goroutine, outside every lock, with the handler's duration measured on the Bus's clock (WithClock; `Now().Sub`) and the handler's result: nil, its error, or panics.ErrPanic (`*panics.PanicError`). A panic in f ends that Subscribe with ErrPanic (the handler's own error, if any, is then not returned: the hook's panic is the later failure, reported first). nil f = no hook (default). When no hook is set the clock is never read (zero cost). |
| `TB-PUBSUB-017` | `WithClock` | `func WithClock[M any](c clock.Clock) Option[M]` | the clock WithOnDelivered measures on; default and nil c = clock.Real(). The Bus never waits on it (no timers). |

**Policy exclusivity (decided here, documented, tested)**: the three slow-subscriber policies are mutually
exclusive and THE LAST OPTION WINS: `NewBus(WithDropOldest(a), WithDropSubscriber(b))` evicts and never drops a
message, `NewBus(WithDropSubscriber(b), WithDropOldest(a))` drops oldest and never evicts. The alternative,
ErrInvalid from NewBus's deferred error when both are set, lost: every other option already follows
last-wins (WithBuffer twice, WithOnSubscribers twice), a Bus that fails on its first Publish for an option
conflict is a surprise at runtime rather than a reading of the option list, and the report of the losing policy
is simply not installed (nothing is silently half-applied). `TestWithDropSubscriber_lastDropOptionWins` pins it.

### Scale note

- Memory bounded: yes. WithDropSubscriber keeps the per-subscriber channel of max(n, 1) (n <= 2^20); eviction is
  one cancel, then one skip per later message until the subscriber's goroutine returns and unregisters; its cost
  is measured by `BenchmarkBus_Publish_dropSubscriber` (below), not claimed. Message
  adds one map per message only when headers are set (nil map otherwise: 0 bytes); the map is shared, not copied
  per subscriber; WithHeader is 1 map clone per call (sized to the source + 1).
- Contention point: unchanged, `Bus.mu` twice per Publish for every topic (bus.go:100-103, :120-122) and the
  per-topic `turn`. This run does not shard. Two benchmarks size it: `BenchmarkBus_Publish_manyTopicsSerial` (64
  topics, ONE publisher round-robin: the per-call cost of the topic map lookup under `Bus.mu` with no contention,
  comparable apples to apples with `BenchmarkBus_Publish`) and `BenchmarkBus_Publish_manyTopicsManyPublishers`
  (64 topics, RunParallel: its ns/op is wall time over all goroutines, i.e. total throughput, NOT the lock's cost;
  it does not isolate `Bus.mu`). Sharding stays an open hardening question until the serial number and a
  contended profile are read together.
- Per-call allocations: Publish 0 allocs/op (measured by `BenchmarkBus_Publish`: the `defer <-t.turn` closure is
  open-coded, not heap-allocated); eviction path: measured by `BenchmarkBus_Publish_dropSubscriber` (below);
  WithOnDelivered: 1 closure per delivery for the panics.Try around f, 2 Now() reads; WithDropSubscriber report:
  1 closure per report (both unmeasured; the hooks are off the default path).
- What breaks first at 10x callers: on one topic, publishers still serialise on `turn` (one at a time); at 100x
  topics `Bus.mu`. What this change fixes: at 10x subscribers of one topic, ONE slow subscriber no longer holds
  the publisher and every later subscriber of that topic (the §6 finding): with WithDropSubscriber the slow one is
  evicted (one cancel) and the fan-out proceeds; the cost is that subscriber's loss (counted, reported).

### Tests (`pubsub/message_test.go`, `drop_subscriber_test.go`, `on_delivered_test.go`, additions to `bus_test.go`, `example_test.go`)

Every test `leak.Verify(t)`; bounded by `bounded(t)` (10 s) or a synctest bubble; no Sleep. Message: Header on nil /
missing / present; WithHeader copy-on-write (receiver unchanged, nil map → new map); headers shared read-only across
subscribers (`TestMessage_withHeaderInOneHandlerDoesNotAffectAnother`); correlation id reaches a handler that
republishes it (`TestMessage_correlationPropagatesThroughHandler`, `ExampleMessage`). WithDropSubscriber: slow
subscriber evicted with ErrSlowSubscriber, the handler's ctx cause, the others receive every message, the
publisher never blocks (synctest), the evicted Subscribe carries the undelivered count (buffer 0 / 1 / 2); Publish
during eviction returns nil and never panics under 4 concurrent publishers × 3 slow subscribers
(`TestWithDropSubscriber_defectHub_closeFromTheHubRace`, the hubs' close-from-the-hub race); report panic → ErrPanic
from Publish, subscriber still evicted; nil report; last drop option wins (both orders); an evicted subscriber gets
nothing more; Close after eviction. WithOnDelivered: called once per delivery with nil / the handler error / ErrPanic;
duration from a clock.Fake advanced inside the handler; hook panic ends the subscription with ErrPanic and the Bus
keeps working; WithClock(nil) is Real. `TestBus_subscribeDuringPublishSeesSnapshot`. Benchmarks (ReportAllocs):
`BenchmarkBus_Publish_manyTopicsSerial`, `BenchmarkBus_Publish_manyTopicsManyPublishers`,
`BenchmarkBus_Publish_dropSubscriber` (every iteration evicts: a fresh subscriber whose handler waits on its ctx,
buffer 1, three Publish calls, the third evicts, then the subscriber's return and unregister are waited for; ns/op
is that whole cycle and the `evictions` metric equals the iteration count, so a number other than that is a bug).

### For the lead

- INDEX rows: TB-PUBSUB-011..017 (regenerate; `doc.go` lists them). taxonomy: pubsub now also depends on `clock`.
- doc/sources: add to `eip_publish_subscribe.md` the "Message" and "Correlation Identifier" pages (011, 012, 013)
  and 014, 015 for "Publish-Subscribe Channel" (policy ownership only; no source for eviction itself); `gamma_1994_design_patterns.md`
  "Observer" + 016; `go_standard_library.md` time Now/Sub + 017.
- D-needed: (1) last-option-wins for the drop policies (above; recommendation last-wins, alternative ErrInvalid);
  (2) the header names `request-id` / `correlation-id` as the names adapters map (lower-case, header-style; the
  alternative `X-Request-ID`-style HTTP casing lost: Kafka/AMQP headers are not HTTP and canonical lower-case
  survives any broker's normalisation; httpx maps them to its HTTP header names).
- ASSUMPTIONS: (8) WithDropSubscriber with no buffer keeps 1 (as WithDropOldest); (9) WithOnDelivered's panic
  replaces the handler's error as Subscribe's result; (10) an evicted subscriber is skipped by later deliveries
  until its goroutine unregisters it (its WithOnSubscribers count drops then, not at eviction).
- ctxkey: `ctxkey.RequestID` lands in the same run (D6). pubsub does NOT import ctxkey: the helper that copies a
  handler ctx's request id into `Message.Headers[HeaderRequestID]` (and back) is a wiring example that belongs to
  httpx (or to the adapters), where both the ctx key and the HTTP header are in scope. The Example here shows the
  propagation by hand (Header + WithHeader).
- calcbench requests: none.

### Measured (VERIFY, re-run after the reviewer fixes, 2026-11-13, go1.27.3 darwin/arm64, -race on; raw in doc/align_2026-11-13/out/bench_final.txt)
BenchmarkBus_Publish 2 298 ns/op, 0 allocs/op (one topic, one draining subscriber, buffer 64). BenchmarkBus_Publish_manyTopicsSerial
2 664 ns/op, 0 allocs/op (64 topics, one publisher round-robin: the per-topic map lookup and turn cost about 0.37 us over the
one-topic case, serial, apples to apples). BenchmarkBus_Publish_manyTopicsManyPublishers 3 553 ns/op, 0 allocs/op (RunParallel
over 64 topics: this ns/op is total wall time across all publisher goroutines, so it measures throughput under contention, not
Bus.mu alone; whether sharding Bus.mu pays is still an open hardening question, the serial number says the lock is not the
dominant cost at 64 topics). BenchmarkBus_Publish_dropSubscriber 24 946 ns/op, 931 B/op, 15 allocs/op with one eviction per
iteration: that is the cost of a full evict-and-resubscribe cycle (a fresh Subscribe goroutine, its subscriber state, the cancel
cause, the report), not of the policy check on a draining subscriber, which the first benchmark shows allocates nothing.
