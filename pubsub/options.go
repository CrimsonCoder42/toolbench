package pubsub

import (
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
)

// maxBuffer is the largest per-subscriber buffer WithBuffer accepts (2^20 messages); above it Publish and
// Subscribe return ErrInvalid instead of allocating without bound.
const maxBuffer = 1 << 20

// Option configures a Bus[M]; pass options to NewBus. It is generic in the Bus's message type, so an option
// for another message type does not compile. The zero set of options is a direct hand-off to each subscriber
// (no buffer), backpressure on a slow subscriber, and no hook. A nil Option is ignored by NewBus.
//
// ID:          TB-PUBSUB-005
// Pattern:     functional options
// Authority:   Pike, "Self-referential functions and the design of options", command center (blog), 2014
// Tags:        pubsub, options
type Option[M any] func(*config[M])

type config[M any] struct {
	buffer      int                                                 // messages that may wait for each subscriber; 0 = direct hand-off
	dropOldest  bool                                                // a full buffer drops its oldest message instead of blocking Publish
	dropSub     bool                                                // a full buffer evicts the subscriber instead of blocking Publish
	report      func(topic string, m M)                             // WithDropOldest's / WithDropSubscriber's report; nil = not reported
	onSubs      func(topic string, subscribers int)                 // WithOnSubscribers hook; nil = none
	onDelivered func(topic string, m M, d time.Duration, err error) // WithOnDelivered hook; nil = none
	clock       clock.Clock                                         // WithClock; nil = clock.Real()
}

// WithBuffer lets up to n messages wait for each subscriber, so Publish can return before a slow subscriber's
// handler has taken them. The default, and any n < 0, is 0: Publish hands each message directly to each
// subscriber and blocks until that subscriber takes it. A subscriber's buffer never grows past n: a full
// buffer blocks Publish (backpressure, no drop) unless WithDropOldest is passed. n above 2^20 (1048576) makes the
// Bus invalid (Publish and Subscribe return ErrInvalid). Each Subscribe allocates its own buffer.
//
// ID:          TB-PUBSUB-006
// Pattern:     bounded queue (backpressure)
// Authority:   Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014, "Stopping short"
// Tags:        pubsub, options, buffer, backpressure
func WithBuffer[M any](n int) Option[M] { return func(c *config[M]) { c.buffer = max(n, 0) } }

// WithDropOldest makes a full subscriber buffer drop its oldest message to take the new one, instead of
// blocking Publish: a slow subscriber then loses its oldest messages and never slows the publisher. Each
// dropped message is passed to report(topic, m) on the publishing goroutine, after the new message has been
// buffered and outside every lock. report must not block for long and must not Publish on the same topic
// (that Publish would wait for the one calling report). A nil report drops without reporting, which the caller
// has then chosen explicitly. The buffer is WithBuffer's n, at least 1 (WithDropOldest with no buffer keeps
// the newest message only). M is inferred from report and must be the Bus's message type (checked by the
// compiler). A panic in report is returned by Publish as panics.ErrPanic. The three slow-subscriber policies
// (backpressure, WithDropOldest, WithDropSubscriber) are mutually exclusive: the last one passed to NewBus wins.
//
// ID:          TB-PUBSUB-007
// Pattern:     ring buffer with overwrite (drop oldest)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Publish-Subscribe Channel"
// Math:        ALG-QUEUE-002 CircularBuffer (Overwrite policy), one per subscriber, under the subscriber's mutex
// Tags:        pubsub, options, drop, ring-buffer, backpressure
func WithDropOldest[M any](report func(topic string, m M)) Option[M] {
	return func(c *config[M]) {
		c.dropOldest, c.dropSub = true, false
		c.report = report
	}
}

// WithDropSubscriber makes a subscriber whose buffer is full when Publish reaches it leave the Bus (the slow
// subscriber is dropped, the messages are not): its ctx is cancelled with cause ErrSlowSubscriber, the message
// that could not be delivered is passed to report(topic, m) on the publishing goroutine outside every lock,
// and Publish goes on to the other subscribers without blocking. The evicted Subscribe returns
// ErrSlowSubscriber (wrapped with the count of messages left in its buffer) once its current handler call has
// returned; later messages skip it. report must not block for long and must not Publish on the same topic. A
// nil report evicts without reporting, which the caller has then chosen explicitly. The buffer is WithBuffer's
// n, at least 1: with no buffer "full" would mean "the handler is between two messages", so one slot is kept,
// as WithDropOldest does. M is inferred from report and must be the Bus's message type. A panic in report is
// returned by Publish as panics.ErrPanic; the eviction has happened. The three slow-subscriber policies
// (backpressure, WithDropOldest, WithDropSubscriber) are mutually exclusive: the last one passed to NewBus wins.
//
// ID:          TB-PUBSUB-015
// Pattern:     publish-subscribe channel (evict the slow subscriber)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Publish-Subscribe Channel" (the delivery policy of a full output channel belongs to the channel; no authority describes eviction itself, see doc/specs/pubsub.md)
// Errors:      ErrSlowSubscriber is the evicted Subscribe's result and its handler ctx's cause; a panic in report is Publish's panics.ErrPanic
// Blocking:    never on the slow subscriber: a full buffer evicts instead of waiting
// Tags:        pubsub, options, drop, evict, slow-subscriber, backpressure
func WithDropSubscriber[M any](report func(topic string, m M)) Option[M] {
	return func(c *config[M]) {
		c.dropSub, c.dropOldest = true, false
		c.report = report
	}
}

// WithOnDelivered calls f(topic, m, d, err) once after each handler call returns, on the subscriber's goroutine
// and outside every lock: d is how long the handler ran, measured on the Bus's clock (WithClock), and err is
// what it returned, nil on success or panics.ErrPanic (a *panics.PanicError) when it panicked. It is the
// per-delivery observability hook (latency histograms, counts, error rates). f must not block for long. A
// panic in f ends that Subscribe with panics.ErrPanic. nil f is no hook (the default), and then the clock is
// never read.
//
// ID:          TB-PUBSUB-016
// Pattern:     observer hook (per delivery)
// Authority:   Gamma, Helm, Johnson, Vlissides, Design Patterns, Addison-Wesley, 1994, "Observer"
// Errors:      a panic in f ends that Subscribe with panics.ErrPanic
// Blocking:    f runs inline on the subscriber's goroutine; the next message waits for it
// Tags:        pubsub, options, hook, observability, latency
func WithOnDelivered[M any](f func(topic string, m M, d time.Duration, err error)) Option[M] {
	return func(c *config[M]) { c.onDelivered = f }
}

// WithClock sets the clock WithOnDelivered measures a handler's duration on; the Bus never waits on it. The
// default, and a nil c, is clock.Real(). Tests pass a clock.Fake and Advance it inside the handler.
//
// ID:          TB-PUBSUB-017
// Pattern:     injected clock (test double seam)
// Authority:   pkg.go.dev time, "func Now" and "func (Time) Sub"
// Tags:        pubsub, options, clock, time, testing
func WithClock[M any](c clock.Clock) Option[M] {
	return func(cfg *config[M]) { cfg.clock = c }
}

// WithOnSubscribers calls f(topic, n) each time the number of subscribers of topic changes: after a Subscribe
// has registered (it then receives every message published from that point) and after one has left. It is the
// observability hook (metrics, or a test waiting until a subscriber is ready). f runs while the Bus holds its
// lock, so calls are in the order of the changes; f must not call the Bus and must not block. A panic in f is
// returned by that Subscribe as panics.ErrPanic. nil f is no hook (the default).
//
// ID:          TB-PUBSUB-008
// Pattern:     observer hook
// Authority:   Gamma, Helm, Johnson, Vlissides, Design Patterns, Addison-Wesley, 1994, "Observer"
// Tags:        pubsub, options, hook, observability
func WithOnSubscribers[M any](f func(topic string, subscribers int)) Option[M] {
	return func(c *config[M]) { c.onSubs = f }
}
