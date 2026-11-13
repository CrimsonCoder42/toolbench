package pubsub

import "maps"

// Message is the envelope a payload travels in: Headers for what the messaging system and its middleware read
// (ids, routing, tracing), Body for the data, carried as-is. It is the agreed type the Bus, the broker adapters
// and the packages built on them speak, so a correlation or request id crosses a process boundary in the
// headers and never inside the payload. The Bus delivers it by value and never reads Headers; the map is SHARED
// read-only among a topic's subscribers, so a handler must not mutate m.Headers: it calls WithHeader, which
// returns a copy. The zero value is ready to use: nil Headers means no headers (Header reports false, WithHeader
// makes a one-entry map). Message[T] is a plain value: no goroutines, nothing blocks, nothing fails.
//
// ID:          TB-PUBSUB-011
// Pattern:     message (envelope: header + body)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Message"
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier"
// Tags:        pubsub, messaging, envelope, headers, correlation
type Message[T any] struct {
	Headers map[string]string // read-only once published; nil = no headers
	Body    T
}

// Header returns the value of key and whether it is present. A nil Headers map has no keys.
func (m Message[T]) Header(key string) (string, bool) {
	v, ok := m.Headers[key]
	return v, ok
}

// WithHeader returns a copy of m whose Headers map is a clone with key set to value; m and the map it shares
// with other subscribers are unchanged (copy-on-write). The clone has at most one entry more than m.Headers.
func (m Message[T]) WithHeader(key, value string) Message[T] {
	h := make(map[string]string, len(m.Headers)+1)
	maps.Copy(h, m.Headers)
	h[key] = value
	m.Headers = h
	return m
}

// HeaderRequestID is the header that carries a request's own identifier. It is the name the broker adapters
// map to their broker's request-id header (lower-case, header-style, as brokers store it), and the name httpx
// maps to the HTTP request id it keeps in the ctx (ctxkey.RequestID).
//
// ID:          TB-PUBSUB-012
// Pattern:     message header (request identifier)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Message"
// Tags:        pubsub, messaging, headers, request-id
const HeaderRequestID = "request-id"

// HeaderCorrelationID is the header that carries the identifier of the request a message answers or follows
// from: a handler that publishes in reaction to a message copies the inbound request (or correlation) id here,
// so every message of one flow can be matched. It is the name the broker adapters map to their broker's
// correlation-id header (lower-case, header-style).
//
// ID:          TB-PUBSUB-013
// Pattern:     correlation identifier (message header)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier"
// Tags:        pubsub, messaging, headers, correlation-id
const HeaderCorrelationID = "correlation-id"
