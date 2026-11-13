package pubsub

import "errors"

// ErrClosed is returned by Publish and Subscribe once Close has been called, and is the cause of the ctx a
// running handler sees when the Bus is closed. Match it with errors.Is.
//
// ID:          TB-PUBSUB-009
// Pattern:     sentinel error
// Authority:   pkg.go.dev net/http, "ErrServerClosed" (the standard library's after-close sentinel)
// Tags:        pubsub, error, closed
var ErrClosed = errors.New("pubsub: bus closed")

// ErrInvalid is returned, wrapped with what it names, by Subscribe for a nil handler, and by Publish and
// Subscribe on a Bus created with WithBuffer above 2^20. Nothing is registered or delivered when it is
// returned. Match it with errors.Is.
//
// ID:          TB-PUBSUB-010
// Pattern:     sentinel error
// Authority:   pkg.go.dev errors, "Is"
// Tags:        pubsub, error, invalid
var ErrInvalid = errors.New("pubsub: invalid argument")

// ErrSlowSubscriber is the cause of a subscriber's ctx, and the result of its Subscribe (wrapped with the count
// of messages left in its buffer), when a Bus created with WithDropSubscriber evicted it because its buffer
// was full at Publish time. Match it with errors.Is.
//
// ID:          TB-PUBSUB-014
// Pattern:     sentinel error
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Publish-Subscribe Channel"
// Tags:        pubsub, error, slow-subscriber, evict
var ErrSlowSubscriber = errors.New("pubsub: slow subscriber")
