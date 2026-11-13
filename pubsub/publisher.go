package pubsub

import "context"

// Publisher publishes messages on a topic. It is the one method a message producer needs; the in-process Bus
// implements it, and so do the broker adapters (adapters/redis, adapters/kafka, adapters/rabbitmq) and the
// packages built on them. Publish returns nil once every message has been handed to the channel (for the Bus:
// to every subscriber registered on topic, in order); an error means some messages may not have been. Publish
// must be safe for concurrent use and must return promptly when ctx is done.
//
// ID:          TB-PUBSUB-001
// Pattern:     publish-subscribe channel (publisher side)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Publish-Subscribe Channel"
// Tags:        pubsub, messaging, interface, publish
type Publisher[M any] interface {
	Publish(ctx context.Context, topic string, msgs ...M) error
}
