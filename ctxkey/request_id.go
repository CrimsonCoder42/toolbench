package ctxkey

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
)

// RequestID is the one key every toolbench package uses for the request / correlation id: logging reads it
// (logging.FromKey), message headers and HTTP and gRPC middleware carry the same value, so one id joins every
// log line, reply and message of one request. A request id is an opaque string of 1 to MaxRequestIDLen bytes,
// every byte printable ASCII (0x21-0x7E: no space, no control character, no CR/LF/NUL, no DEL, no non-ASCII),
// so it passes a header, a log line and a message header unchanged. An inbound id (an HTTP header, a message
// header) is attacker text: store it through WithRequestID, which validates, never through RequestID.With
// directly; EnsureRequestID replaces a stored id that does not pass ValidRequestID. The key is immutable and
// safe for concurrent use; it prints as "ctxkey.request-id". RequestID is a package-level identity, like a
// sentinel error (io.EOF): read it, never assign to it. Reassigning it makes a different key, and the id would
// then be split between the packages that read the old pointer and those that read the new one.
//
// ID:          TB-CTXKEY-003
// Pattern:     correlation identifier (typed context key)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier"
// Authority:   pkg.go.dev, package context, "func WithValue", go1.27
// Tags:        context, request-id, correlation-id, key
var RequestID = New[string]("ctxkey.request-id")

// MaxRequestIDLen is the longest request id, in bytes, that ValidRequestID accepts and WithRequestID stores:
// 128. A UUID (36), a W3C traceparent (55) and a host-counter id fit with margin; a 10 KB inbound header does
// not and is rejected rather than stored, logged or echoed.
//
// ID:          TB-CTXKEY-004
// Pattern:     bounded input (default bound)
// Authority:   RFC 9110, HTTP Semantics, 2022, §5.5 "Field Values"
// Tags:        request-id, bound, default
const MaxRequestIDLen = 128

// ErrRequestID is returned, wrapped, by WithRequestID when the id is empty, longer than MaxRequestIDLen or
// holds a byte outside printable ASCII 0x21-0x7E; the context was not changed. The wrapped message names the
// reason and the byte length, never the id's bytes, so logging the error cannot forge a log line. Match it
// with errors.Is.
//
// ID:          TB-CTXKEY-005
// Pattern:     sentinel error
// Authority:   RFC 9110, HTTP Semantics, 2022, §5.5 "Field Values" (CR, LF and NUL in a field value are invalid and dangerous)
// Tags:        request-id, error, sentinel, validation
var ErrRequestID = errors.New("ctxkey: invalid request id")

// NewRequestID returns a fresh request id: crypto/rand.Text, 26 characters of the RFC 4648 base32 alphabet
// (A-Z, 2-7) carrying at least 128 bits of randomness, so ids are distinct with vanishing collision
// probability and never guessable. Every id it returns passes ValidRequestID and fits a header, a URL and a log
// line without escaping. There is no seed, no counter and no shared state: call it from any goroutine. It is
// the choice over a UUID because an id is not mathematics (calcbench has no identifier primitive) and the
// standard library's Text is shorter and carries no version semantics; a consumer that must emit a UUID stores
// its own through WithRequestID.
//
// ID:          TB-CTXKEY-006
// Pattern:     correlation identifier (generation)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier"
// Authority:   pkg.go.dev, package crypto/rand, "func Text", go1.24
// Errors:      none: crypto/rand never returns an error; an unusable OS random source crashes the program by that package's contract, never a weak id
// Blocking:    none (one read of the OS random source)
// Tags:        request-id, correlation-id, random, generator
func NewRequestID() string {
	return rand.Text()
}

// ValidRequestID reports whether s is a request id this package stores and forwards: not empty, at most
// MaxRequestIDLen bytes, and every byte in 0x21-0x7E (printable ASCII: no space, no tab, no CR, LF or NUL, no
// DEL, no non-ASCII byte). The check is byte-wise, so any multi-byte rune is rejected. Call it on every inbound
// id (an HTTP header, a message header, a query parameter) before the id reaches a log or a context. Pure, no
// allocation; the length is checked before the bytes, so a huge input costs one compare.
//
// ID:          TB-CTXKEY-007
// Pattern:     input validation (allow-list)
// Authority:   RFC 9110, HTTP Semantics, 2022, §5.5 "Field Values" (visible ASCII; CR, LF and NUL are invalid and dangerous)
// Tags:        request-id, validation, header, log-forging
func ValidRequestID(s string) bool {
	if len(s) == 0 || len(s) > MaxRequestIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7E {
			return false
		}
	}
	return true
}

// WithRequestID stores id under RequestID in a child of ctx after validating it with ValidRequestID. An invalid
// id (empty, over MaxRequestIDLen, a byte outside 0x21-0x7E) returns ctx unchanged and ErrRequestID wrapped
// with the reason and the byte length (never the bytes): nothing is stored, so an attacker's header is neither
// logged nor echoed. A nil ctx is treated as context.Background on both paths, so the returned context is never
// nil. Starts no goroutine, never blocks, never panics.
//
// ID:          TB-CTXKEY-008
// Pattern:     correlation identifier (validated store)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier"
// Authority:   RFC 9110, HTTP Semantics, 2022, §5.5 "Field Values"
// Errors:      ErrRequestID (wrapped) when id fails ValidRequestID; ctx is returned unchanged
// Tags:        request-id, correlation-id, context, validation
func WithRequestID(ctx context.Context, id string) (context.Context, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch {
	case len(id) == 0:
		return ctx, fmt.Errorf("%w: empty", ErrRequestID)
	case len(id) > MaxRequestIDLen:
		return ctx, fmt.Errorf("%w: %d bytes, limit %d", ErrRequestID, len(id), MaxRequestIDLen)
	case !ValidRequestID(id):
		return ctx, fmt.Errorf("%w: byte outside printable ASCII (%d bytes)", ErrRequestID, len(id))
	}
	return RequestID.With(ctx, id), nil
}

// RequestIDFrom returns the request id stored under RequestID in ctx and true, or "" and false when none is
// stored or ctx is nil. It returns what is stored, even an id placed by RequestID.With without validation; a
// caller forwarding the id to a header uses EnsureRequestID, which replaces an invalid one. Never blocks.
//
// ID:          TB-CTXKEY-009
// Pattern:     correlation identifier (typed read)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier"
// Authority:   pkg.go.dev, package context, "type Context" (method Value), go1.27
// Tags:        request-id, correlation-id, context, read
func RequestIDFrom(ctx context.Context) (string, bool) {
	return RequestID.Value(ctx)
}

// EnsureRequestID returns ctx and its request id when ctx already holds one that passes ValidRequestID,
// allocating nothing; otherwise (no id, or a stored id that is not valid) it mints one with NewRequestID,
// stores it under RequestID in a child of ctx and returns that child and the id. A nil ctx is treated as
// context.Background. It never fails: a middleware calls it once at the boundary and every later read sees
// the same id. Starts no goroutine, never blocks, never panics.
//
// ID:          TB-CTXKEY-010
// Pattern:     correlation identifier (get-or-create at the boundary)
// Authority:   Hohpe & Woolf, Enterprise Integration Patterns, Addison-Wesley, 2003, "Correlation Identifier"
// Authority:   pkg.go.dev, package crypto/rand, "func Text", go1.24
// Tags:        request-id, correlation-id, context, middleware
func EnsureRequestID(ctx context.Context) (context.Context, string) {
	if id, ok := RequestID.Value(ctx); ok && ValidRequestID(id) {
		return ctx, id
	}
	id := NewRequestID()
	return RequestID.With(ctx, id), id
}
