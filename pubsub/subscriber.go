package pubsub

import "context"

// Subscriber calls h for each message on topic, one at a time, in the channel's order, until ctx is done, the
// channel closes or h returns an error. It blocks for the whole subscription and returns the reason it ended,
// never nil: context.Cause(ctx), ErrClosed, or h's error (a panic in h as panics.ErrPanic), always after h has
// returned. Unsubscribing is cancelling ctx. The in-process Bus implements it, and so do the broker adapters.
// What happens to a message whose h failed (lost, redelivered, dead-lettered) is the implementation's
// documented choice; for the Bus it is lost (at-most-once).
//
// ID:          TB-PUBSUB-002
// Pattern:     event-driven consumer (on a publish-subscribe channel)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Event-Driven Consumer"
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Publish-Subscribe Channel"
// Tags:        pubsub, messaging, interface, subscribe, consumer
type Subscriber[M any] interface {
	Subscribe(ctx context.Context, topic string, h func(context.Context, M) error) error
}
