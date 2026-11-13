package pubsub

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/queue"
	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// subscriber is one Subscribe call's delivery state. Exactly one of ch (backpressure and drop-subscriber) and
// ring (drop oldest) is set. Its handler loop runs on the Subscribe caller's goroutine; publishers only add to it.
type subscriber[M any] struct {
	topic  string
	ctx    context.Context // Subscribe's ctx; cancelled by Close (cause ErrClosed), by an eviction (ErrSlowSubscriber) and when Subscribe returns
	cancel context.CancelCauseFunc

	// mu: a backpressure Publish holds it for reading while it may send on ch; a drop-oldest Publish holds it
	// to push; close takes it to set closed, so no message lands after the undelivered count is taken.
	mu     sync.RWMutex
	closed bool                     // guarded by mu
	ch     chan M                   // capacity = WithBuffer's n (at least 1 with evict); never closed (publishers stop on closed/ctx)
	ring   *queue.CircularBuffer[M] // guarded by mu; ALG-QUEUE-002, policy Overwrite
	notify chan struct{}            // capacity 1: a push into ring wakes the handler loop
	evict  bool                     // WithDropSubscriber: a full ch evicts s instead of blocking
	report func(topic string, m M)  // WithDropOldest's / WithDropSubscriber's report; may be nil

	onDelivered func(topic string, m M, d time.Duration, err error) // WithOnDelivered; may be nil
	clock       clock.Clock                                         // read only when onDelivered is set
}

func newSubscriber[M any](ctx context.Context, topic string, c config[M]) (*subscriber[M], error) {
	sctx, cancel := context.WithCancelCause(ctx)
	s := &subscriber[M]{topic: topic, ctx: sctx, cancel: cancel, onDelivered: c.onDelivered, clock: c.clock}
	if s.clock == nil {
		s.clock = clock.Real()
	}
	switch {
	case c.dropSub:
		s.ch, s.evict, s.report = make(chan M, max(c.buffer, 1)), true, c.report
		return s, nil
	case !c.dropOldest:
		s.ch = make(chan M, c.buffer)
		return s, nil
	}
	ring, err := queue.NewCircularBuffer[M](max(c.buffer, 1), queue.Overwrite)
	if err != nil { // only ErrTooLarge for a huge M; the count is already bounded by maxBuffer
		cancel(nil)
		return nil, fmt.Errorf("%w: WithDropOldest buffer: %w", ErrInvalid, err)
	}
	s.ring, s.notify, s.report = ring, make(chan struct{}, 1), c.report
	return s, nil
}

// deliver hands m to s. It returns nil when s took m or has left (a subscriber that left, or was evicted, is
// skipped), and an error only when the publisher must stop: its ctx ended, the Bus closed, or report panicked.
func (s *subscriber[M]) deliver(ctx context.Context, done <-chan struct{}, topic string, m M) error {
	if s.ring != nil {
		return s.push(topic, m)
	}
	if s.evict {
		return s.offer(done, topic, m)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil
	}
	select {
	case s.ch <- m:
		return nil
	case <-s.ctx.Done(): // the subscriber is leaving: skip it, unless it is leaving because the Bus closed
		select {
		case <-done:
			return ErrClosed
		default:
			return nil
		}
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-done:
		return ErrClosed
	}
}

// offer is the drop-subscriber policy: a non-blocking send; a full ch evicts s (its ctx cancelled with cause
// ErrSlowSubscriber) and then reports m, outside every lock. It never blocks on the subscriber. A subscriber
// that already left or was evicted is skipped.
func (s *subscriber[M]) offer(done <-chan struct{}, topic string, m M) error {
	s.mu.RLock()
	if s.closed || s.ctx.Err() != nil {
		s.mu.RUnlock()
		select {
		case <-done:
			return ErrClosed
		default:
			return nil
		}
	}
	select {
	case s.ch <- m:
		s.mu.RUnlock()
		return nil
	default:
	}
	s.cancel(ErrSlowSubscriber) // the handler loop sees it after its current call; the buffer is counted by close
	s.mu.RUnlock()
	if s.report == nil {
		return nil
	}
	return panics.Try(func() error { s.report(topic, m); return nil })
}

// push adds m to the ring, dropping (and then reporting) the oldest message when the ring is full. It never
// blocks on the subscriber.
func (s *subscriber[M]) push(topic string, m M) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	oldest, hadFront := s.ring.Front() // read before Push: Push reports only that it overwrote
	overwritten, _ := s.ring.Push(m)   // Overwrite on a capacity >= 1 ring never fails
	dropped := overwritten && hadFront
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default: // a wake-up is already pending
	}
	if !dropped || s.report == nil {
		return nil
	}
	return panics.Try(func() error { s.report(topic, oldest); return nil })
}

// run is the event-driven consumer loop: it calls h for each message until s.ctx is done or h fails. ctx is
// checked before each wait, so a cancel seen there stops the loop; but a message taken in the same instant as
// the cancel (next's select may pick either) is still handed to h once, with the done ctx, rather than lost
// uncounted.
func (s *subscriber[M]) run(h func(context.Context, M) error) error {
	for {
		if s.ctx.Err() != nil {
			return context.Cause(s.ctx)
		}
		m, ok, err := s.next()
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := s.handle(h, m); err != nil {
			return err
		}
	}
}

// handle calls h once for m, a panic recovered as panics.ErrPanic, and then the WithOnDelivered hook with the
// duration and h's result, outside every lock. A panic in the hook is the error returned (it is the later
// failure); otherwise h's.
func (s *subscriber[M]) handle(h func(context.Context, M) error, m M) error {
	if s.onDelivered == nil {
		return panics.Try(func() error { return h(s.ctx, m) })
	}
	start := s.clock.Now()
	err := panics.Try(func() error { return h(s.ctx, m) })
	d := s.clock.Now().Sub(start)
	if herr := panics.Try(func() error { s.onDelivered(s.topic, m, d, err); return nil }); herr != nil {
		return herr
	}
	return err
}

// next waits for the next message. ok is false when it was woken without one (re-check ctx and retry).
func (s *subscriber[M]) next() (m M, ok bool, err error) {
	if s.ring == nil {
		select {
		case m = <-s.ch:
			return m, true, nil
		case <-s.ctx.Done():
			return m, false, context.Cause(s.ctx)
		}
	}
	s.mu.Lock()
	m, ok = s.ring.Pop()
	s.mu.Unlock()
	if ok {
		return m, true, nil
	}
	select {
	case <-s.notify:
		return m, false, nil
	case <-s.ctx.Done():
		return m, false, context.Cause(s.ctx)
	}
}

// close marks s closed, so no publisher adds to it any more, empties its buffer (so the messages are not
// retained) and returns how many messages were left undelivered. s.ctx must be cancelled first, so that every
// publisher blocked on s returns and releases s.mu.
func (s *subscriber[M]) close() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	n := 0
	if s.ring != nil {
		for _, ok := s.ring.Pop(); ok; _, ok = s.ring.Pop() {
			n++
		}
		return n
	}
	for {
		select {
		case <-s.ch:
			n++
		default:
			return n
		}
	}
}
