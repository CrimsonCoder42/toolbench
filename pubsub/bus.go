package pubsub

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/CrimsonCoder42/toolbench/panics"
)

// Bus is an in-process publish-subscribe channel: Publish delivers a copy of each message to every subscriber
// registered on its topic when the message is published, and Subscribe calls a handler for each message of its
// topic on the caller's goroutine. It implements Publisher and Subscriber. Delivery is at-most-once: a message
// is lost for a subscriber that leaves before handling it (counted in that Subscribe's error) or, with
// WithDropOldest, that falls behind (reported). Publish with no subscriber on the topic delivers nothing and
// returns nil. Every subscriber of a topic sees that topic's messages in one order, the order Publish calls
// were admitted, each call's messages consecutively. Topics are independent and created on first Subscribe.
// The zero value is ready to use (no buffer, backpressure, no hook); NewBus sets options. A Bus must not be
// copied after first use.
//
// ID:          TB-PUBSUB-003
// Pattern:     publish-subscribe channel (event-driven consumers)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Publish-Subscribe Channel"
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Event-Driven Consumer"
// Goroutines:  none; Subscribe runs on the caller's goroutine and has returned, its handler finished, when it returns
// Cancel:      Publish returns context.Cause(ctx) when its ctx ends while it waits (messages already handed over stay delivered); Subscribe returns context.Cause(ctx) after its current handler call
// Errors:      ErrClosed after Close; ErrInvalid for a nil handler or invalid options; ErrSlowSubscriber for a subscriber evicted by WithDropSubscriber; a handler error or panic (panics.ErrPanic) ends that Subscribe and is returned
// Blocking:    Publish blocks while a subscriber of the topic is busy and its buffer is full (backpressure), unless WithDropOldest or WithDropSubscriber; Subscribe blocks for the whole subscription
// Math:        ALG-QUEUE-002 CircularBuffer (WithDropOldest only)
// Tags:        concurrency, pubsub, messaging, fan-out, backpressure, panic
type Bus[M any] struct {
	cfg config[M]
	err error // invalid options, found by NewBus; returned by Publish and Subscribe

	mu     sync.Mutex           // guards topics, closed, done (lazily made)
	topics map[string]*topic[M] // a topic exists while it has at least one subscriber
	closed bool
	done   chan struct{} // closed by Close: wakes every Publish waiting on a subscriber
}

// topic is the subscriber list of one topic and the turn that orders its publishers.
type topic[M any] struct {
	turn chan struct{}    // capacity 1: held by one Publish at a time, so every subscriber sees one order
	subs []*subscriber[M] // copy-on-write under Bus.mu; Publish reads a snapshot
}

// NewBus returns a Bus configured by opts. A nil Option is ignored. WithBuffer above 2^20 does not panic: the
// Bus is returned and its Publish and Subscribe return ErrInvalid naming the option.
//
// ID:          TB-PUBSUB-004
// Pattern:     publish-subscribe channel (constructor)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Publish-Subscribe Channel"
// Blocking:    never
// Tags:        pubsub, constructor
func NewBus[M any](opts ...Option[M]) *Bus[M] {
	b := &Bus[M]{}
	for _, o := range opts {
		if o != nil {
			o(&b.cfg)
		}
	}
	if b.cfg.buffer > maxBuffer {
		b.err = fmt.Errorf("%w: WithBuffer(%d) is above %d", ErrInvalid, b.cfg.buffer, maxBuffer)
	}
	return b
}

// lazy makes the zero value usable. Called with b.mu held.
func (b *Bus[M]) lazy() {
	if b.done == nil {
		b.done = make(chan struct{})
		b.topics = map[string]*topic[M]{}
	}
}

// Publish delivers msgs, in order, to every subscriber registered on topic, each message to all of them before
// the next. It returns nil once every subscriber has taken (or buffered) every message; with no subscriber on
// topic, or no msgs, it delivers nothing and returns nil. It blocks while a subscriber is busy and its buffer
// is full (backpressure), unless WithDropOldest (drops its oldest message) or WithDropSubscriber (evicts it with
// cause ErrSlowSubscriber); a subscriber that leaves while Publish waits on it, or was evicted, is skipped.
// Concurrent Publish calls on one topic take turns (one order for every subscriber); different topics never
// wait for each other. It returns ErrClosed after Close (also while waiting), context.Cause(ctx) when ctx
// ends first, ErrInvalid for invalid options, and a panic in a WithDropOldest or WithDropSubscriber report as
// panics.ErrPanic; on an
// error the messages already handed over stay delivered and the rest are not published. A nil ctx is
// context.Background(). Safe for concurrent use, also from a handler (on another topic, or with buffer to
// spare: a handler publishing to its own full, unbuffered topic waits for itself).
func (b *Bus[M]) Publish(ctx context.Context, topic string, msgs ...M) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if b == nil {
		return ErrClosed
	}
	if b.err != nil {
		return b.err
	}
	b.mu.Lock()
	b.lazy()
	closed, done, t := b.closed, b.done, b.topics[topic]
	b.mu.Unlock()
	switch {
	case closed:
		return ErrClosed
	case ctx.Err() != nil:
		return context.Cause(ctx)
	case t == nil || len(msgs) == 0:
		return nil
	}
	select {
	case t.turn <- struct{}{}:
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-done:
		return ErrClosed
	}
	defer func() { <-t.turn }()
	b.mu.Lock()
	closed, subs := b.closed, t.subs // a topic deleted meanwhile has no subs: its subscribers have all left
	b.mu.Unlock()
	if closed {
		return ErrClosed
	}
	for _, m := range msgs {
		for _, s := range subs {
			if err := s.deliver(ctx, done, topic, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// Subscribe registers h on topic and calls it for each message published there from then on, one at a time on
// the calling goroutine, in the topic's order, with a ctx that is cancelled when ctx is or when the Bus closes
// (cause ErrClosed). It blocks until that ctx is done or h returns an error, and returns after h has returned;
// it never returns nil. The result is context.Cause(ctx), ErrClosed, ErrSlowSubscriber (evicted by
// WithDropSubscriber), h's error, a panic in h or in a WithOnDelivered hook as panics.ErrPanic; when messages
// were left in this subscriber's buffer (lost: at-most-once) it is wrapped as
// "pubsub: N buffered messages not delivered: <reason>", so errors.Is still matches the reason. Unsubscribe by
// cancelling ctx. It returns at once ErrClosed on a closed Bus, ErrInvalid for a nil h or invalid options, and
// context.Cause(ctx) when ctx is already done. A nil ctx is context.Background(). Safe for concurrent use; one
// topic may have any number of subscribers, each with its own buffer.
func (b *Bus[M]) Subscribe(ctx context.Context, topic string, h func(context.Context, M) error) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch {
	case b == nil:
		return ErrClosed
	case b.err != nil:
		return b.err
	case h == nil:
		return fmt.Errorf("%w: nil handler for topic %q", ErrInvalid, topic)
	case ctx.Err() != nil:
		return context.Cause(ctx)
	}
	s, err := newSubscriber[M](ctx, topic, b.cfg)
	if err != nil {
		return err
	}
	defer s.cancel(nil)
	if err := b.register(topic, s); err != nil {
		return err
	}
	defer func() {
		s.cancel(nil) // wakes every Publish waiting on s before s.mu is taken
		err = errors.Join(err, b.unregister(topic, s))
		if n := s.close(); n > 0 {
			err = fmt.Errorf("pubsub: %d buffered messages not delivered: %w", n, err)
		}
	}()
	return s.run(h)
}

// register adds s to topic, or returns ErrClosed. A panic in the hook unregisters s and is returned.
func (b *Bus[M]) register(name string, s *subscriber[M]) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lazy()
	if b.closed {
		return ErrClosed
	}
	t := b.topics[name]
	if t == nil {
		t = &topic[M]{turn: make(chan struct{}, 1)}
		b.topics[name] = t
	}
	t.subs = append(slices.Clip(t.subs), s)
	if err := b.hook(name, len(t.subs)); err != nil {
		b.remove(name, t, s)
		return err
	}
	return nil
}

// unregister removes s from topic and calls the hook; its error is a panic in the hook.
func (b *Bus[M]) unregister(name string, s *subscriber[M]) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.topics[name]
	if t == nil || !slices.Contains(t.subs, s) {
		return nil
	}
	n := b.remove(name, t, s)
	return b.hook(name, n)
}

// remove takes s out of t (copy-on-write) and deletes t when it is empty. Called with b.mu held.
func (b *Bus[M]) remove(name string, t *topic[M], s *subscriber[M]) int {
	t.subs = slices.DeleteFunc(slices.Clone(t.subs), func(x *subscriber[M]) bool { return x == s })
	if len(t.subs) == 0 {
		delete(b.topics, name)
	}
	return len(t.subs)
}

// hook runs WithOnSubscribers' f, a panic returned as panics.ErrPanic. Called with b.mu held.
func (b *Bus[M]) hook(name string, n int) error {
	if b.cfg.onSubs == nil {
		return nil
	}
	return panics.Try(func() error { b.cfg.onSubs(name, n); return nil })
}

// Close closes the Bus: Publish and Subscribe return ErrClosed from now on, a Publish waiting on a subscriber
// returns ErrClosed, and every running Subscribe's ctx is cancelled with cause ErrClosed, so each returns
// ErrClosed once its current handler call has returned (Close does not wait for that: the Subscribe callers
// own their goroutines). Messages still buffered are not delivered and are counted in each Subscribe's error.
// Close always returns nil; calling it again, or on the zero value or a nil *Bus, is a no-op. It may be called
// from a handler.
func (b *Bus[M]) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lazy()
	if b.closed {
		return nil
	}
	b.closed = true
	close(b.done)
	for _, t := range b.topics {
		for _, s := range t.subs {
			s.cancel(ErrClosed)
		}
	}
	return nil
}

var (
	_ Publisher[int]  = (*Bus[int])(nil)
	_ Subscriber[int] = (*Bus[int])(nil)
)
