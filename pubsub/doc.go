// Package pubsub is in-process publish-subscribe: a Bus delivers a copy of every message published on a topic
// to each subscriber of that topic (Publish-Subscribe Channel), by calling the subscriber's handler on the
// subscriber's own goroutine (Event-Driven Consumer). Publisher and Subscriber are the two small interfaces the
// Bus implements and that later packages (jobs, outbox) and the adapters (Redis, Kafka, RabbitMQ) implement for
// an outside broker.
//
// Delivery is AT-MOST-ONCE inside the process: a message reaches each subscriber registered when it is published
// at most once, and is lost when the subscriber leaves (ctx cancelled, handler error, Close) before handling it;
// those losses are never silent: Subscribe's error counts them, and WithDropOldest reports each message it
// drops. A slow subscriber slows Publish (backpressure) unless WithDropOldest is passed. Every subscriber of a
// topic sees that topic's messages in the same order. The Bus starts no goroutine: Subscribe runs its loop on
// the caller's goroutine and returns when its ctx is done, the Bus is closed or the handler fails, with the
// handler finished. A panic in a handler is returned as panics.ErrPanic. Nothing panics on input.
//
// ID short form: PUBSUB (IDs are TB-PUBSUB-NNN).
//
// Exports:
//
//	TB-PUBSUB-001  Publisher          publishes messages on a topic (the interface the Bus and broker adapters implement)
//	TB-PUBSUB-002  Subscriber         calls a handler for each message on a topic until ctx is done (the interface the Bus and broker adapters implement)
//	TB-PUBSUB-003  Bus                an in-process publish-subscribe channel: a copy of each message to every subscriber of its topic
//	TB-PUBSUB-004  NewBus             creates a Bus with options
//	TB-PUBSUB-005  Option             a functional option for NewBus
//	TB-PUBSUB-006  WithBuffer         how many messages may wait for each subscriber (default 0: direct hand-off)
//	TB-PUBSUB-007  WithDropOldest     a full subscriber buffer drops its oldest message and reports it, instead of blocking Publish
//	TB-PUBSUB-008  WithOnSubscribers  a hook called with a topic's subscriber count each time it changes
//	TB-PUBSUB-009  ErrClosed          returned by Publish and Subscribe once the Bus is closed
//	TB-PUBSUB-010  ErrInvalid         returned when a Bus is used with an argument or option it cannot run with
//	TB-PUBSUB-011  Message            the envelope: Headers (shared read-only; WithHeader copies) and Body, the type adapters speak
//	TB-PUBSUB-012  HeaderRequestID    the "request-id" header name adapters map to their broker's request-id header
//	TB-PUBSUB-013  HeaderCorrelationID  the "correlation-id" header name adapters map to their broker's correlation-id header
//	TB-PUBSUB-014  ErrSlowSubscriber  the cause and result of a Subscribe evicted by WithDropSubscriber
//	TB-PUBSUB-015  WithDropSubscriber  a full subscriber buffer evicts that subscriber (reported) instead of blocking Publish
//	TB-PUBSUB-016  WithOnDelivered    a hook called after each handler call with the topic, message, duration and error
//	TB-PUBSUB-017  WithClock          the clock WithOnDelivered measures on (default clock.Real())
package pubsub
