package ctxkey

import (
	"context"
	"fmt"
)

// Key is a typed key for a request-scoped value of type T carried in a context.Context.
//
// The key's identity is its pointer: two keys made by two New calls are distinct even when their names and
// types are equal, so a value stored under one is never seen through the other, and no key can ever equal a
// string or any other built-in key another package uses. Key has a string field, so it is never zero-size and
// two keys never share an address. A Key is immutable and safe for concurrent use. Create it with New and keep
// it in a package-level variable; the zero value used through a pointer is a valid key with the empty name, and
// copying a Key makes a different key.
//
// ID:          TB-CTXKEY-001
// Pattern:     typed context key (type-safe accessors for request-scoped values)
// Authority:   pkg.go.dev, package context, "func WithValue" and "type Context" (method Value), go1.27
// Authority:   Ajmani, "Go Concurrency Patterns: Context", The Go Blog, 2014, "Example: Google Web Search"
// Tags:        context, request-scoped, type-safe, key
type Key[T any] struct {
	name string
}

// New returns a new key for values of type T. The name is shown when the key or a context holding it is
// printed and is never part of the key's identity: New[int]("user") twice gives two distinct keys. Callers
// namespace the name ("auth.user-id") so printed contexts stay readable.
//
// ID:          TB-CTXKEY-002
// Pattern:     typed context key (type-safe accessors for request-scoped values)
// Authority:   pkg.go.dev, package context, "func WithValue" and "type Context" (method Value), go1.27
// Authority:   Ajmani, "Go Concurrency Patterns: Context", The Go Blog, 2014, "Example: Google Web Search"
// Tags:        context, request-scoped, type-safe, key, constructor
func New[T any](name string) *Key[T] {
	return &Key[T]{name: name}
}

// With returns a child of ctx in which k holds v; ctx itself is not changed. A later With on the same key in a
// descendant context shadows v for that descendant only. A nil ctx is treated as context.Background() (the
// standard library panics there; a toolbench function never panics on input). On a nil k, With stores nothing
// and returns ctx unchanged. With starts no goroutine and never blocks; cancellation is ctx's.
func (k *Key[T]) With(ctx context.Context, v T) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if k == nil {
		return ctx
	}
	return context.WithValue(ctx, k, entry[T]{v: v})
}

// Value returns the value k holds in ctx (the nearest one, walking from ctx to its root) and true, or the zero
// value of T and false when k holds nothing there. A stored nil (for example a nil error) reads back as
// (nil, true). A nil ctx or a nil k gives (zero, false). Value never blocks.
func (k *Key[T]) Value(ctx context.Context) (T, bool) {
	if ctx == nil || k == nil {
		var zero T
		return zero, false
	}
	e, ok := ctx.Value(k).(entry[T])
	return e.v, ok
}

// String returns the key's name, so fmt.Sprint of a context shows it, as in
// "context.Background.WithValue(user-id, int)". A nil k prints "<nil>".
func (k *Key[T]) String() string {
	if k == nil {
		return "<nil>"
	}
	return k.name
}

// entry boxes the stored value so that a stored nil interface value is told apart from a missing one.
type entry[T any] struct {
	v T
}

// String prints the value the way the context package prints a WithValue value: a fmt.Stringer's String, a
// string as itself, nil as "<nil>", anything else only as its type name, so printing a context never prints a
// non-string value.
func (e entry[T]) String() string {
	// Written as explicit assertions, not a type switch: go1.27.3 crashes with an internal compiler error
	// ("ctxkey/key.go: invalid memory address or nil pointer dereference") when a type switch here is compiled
	// under -race into a test binary that instantiates Key with a pointer type (pool's tests, align run
	// 2026-11-13). The behaviour is unchanged.
	v := any(e.v)
	if s, ok := v.(fmt.Stringer); ok {
		return s.String()
	}
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", e.v)
}
