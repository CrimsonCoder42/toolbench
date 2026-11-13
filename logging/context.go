package logging

import (
	"context"
	"errors"
	"log/slog"

	"github.com/CrimsonCoder42/toolbench/ctxkey"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// Context wraps h so that the attributes extract returns for a record's context are added to that record, at
// the top level and first: before the logger's With attributes and outside any group the logger opened, so a
// request id is always found at the same path. To keep that true under WithGroup, the wrapper keeps the
// WithAttrs/WithGroup chain (in order) beside a copy of h with the chain applied; a record whose context gives
// attributes is handled by h.WithAttrs(those) with the chain replayed (cost: the chain is re-applied for that
// record), a record whose context gives none by the pre-applied copy. Enabled is h's. A nil ctx in Handle is
// context.Background(); a nil extract returns h; a nil h is slog.DiscardHandler. Wrap Redact inside it,
// Context(Redact(out), extract), so extracted values are redacted. Safe for concurrent use.
//
// ID:          TB-LOG-006
// Pattern:     decorator (wrapping slog.Handler) adding request-scoped context attributes
// Authority:   Amsterdam, "Structured Logging with slog", The Go Blog, 2023 ("a handler can extract context information like trace IDs")
// Authority:   Go team, "A Guide to Writing slog Handlers", golang/example slog-handler-guide, "Implementing WithGroup and WithAttrs"
// Errors:      h's Handle error; a panic in extract is recovered (panics.ErrPanic), the record is still passed to h without context attributes, and both errors are joined
// Blocking:    only as much as extract and h's Handle
// Tags:        logging, slog, context, request-scoped, handler, decorator
func Context(h slog.Handler, extract func(context.Context) []slog.Attr) slog.Handler {
	if h == nil {
		h = slog.DiscardHandler
	}
	if extract == nil {
		return h
	}
	return &contextHandler{base: h, applied: h, extract: extract}
}

// step is one WithAttrs (attrs) or WithGroup (group) call, kept in order (the guide's groupOrAttrs).
type step struct {
	group string
	attrs []slog.Attr
}

// contextHandler is the handler Context returns. base is the wrapped handler as given; applied is base with
// chain applied. All fields are immutable after construction.
type contextHandler struct {
	base    slog.Handler
	applied slog.Handler
	chain   []step
	extract func(context.Context) []slog.Attr
}

func (h *contextHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.applied.Enabled(ctx, l)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx == nil {
		ctx = context.Background()
	}
	attrs, perr := panics.TryValue(func() ([]slog.Attr, error) { return h.extract(ctx), nil })
	if perr != nil || len(attrs) == 0 {
		return errors.Join(perr, h.applied.Handle(ctx, r))
	}
	if len(h.chain) == 0 {
		nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
		nr.AddAttrs(attrs...)
		r.Attrs(func(a slog.Attr) bool {
			nr.AddAttrs(a)
			return true
		})
		return h.base.Handle(ctx, nr)
	}
	t := h.base.WithAttrs(attrs)
	for _, s := range h.chain {
		if s.group != "" {
			t = t.WithGroup(s.group)
		} else {
			t = t.WithAttrs(s.attrs)
		}
	}
	return t.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.with(step{attrs: append([]slog.Attr(nil), attrs...)}, h.applied.WithAttrs(attrs))
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.with(step{group: name}, h.applied.WithGroup(name))
}

// with returns a new handler with s appended to a copy of the chain (the receiver is never modified).
func (h *contextHandler) with(s step, applied slog.Handler) *contextHandler {
	chain := make([]step, len(h.chain), len(h.chain)+1)
	copy(chain, h.chain)
	return &contextHandler{base: h.base, applied: applied, chain: append(chain, s), extract: h.extract}
}

// FromKey returns an extract func for Context that reads k from the context and gives one attribute, name =
// the value, when k holds a value there, and nothing otherwise. A value that is a slog.LogValuer is resolved
// by the output handler (and by Redact). A nil k gives nothing.
//
// ID:          TB-LOG-007
// Pattern:     typed context key to log attribute (request-scoped values in logs)
// Authority:   pkg.go.dev log/slog, "func Any" and "type LogValuer"; pkg.go.dev context, "type Context" (method Value)
// Tags:        logging, slog, context, ctxkey, request-scoped
func FromKey[T any](name string, k *ctxkey.Key[T]) func(context.Context) []slog.Attr {
	return func(ctx context.Context) []slog.Attr {
		v, ok := k.Value(ctx)
		if !ok {
			return nil
		}
		return []slog.Attr{slog.Any(name, v)}
	}
}

// Combine returns one extract func that calls each of extracts in order and concatenates their attributes;
// nil funcs are skipped. Use it to pass several FromKey funcs to Context.
//
// ID:          TB-LOG-008
// Pattern:     composite (several extractors as one)
// Authority:   Gamma et al., Design Patterns, 1994, "Composite"
// Tags:        logging, slog, context, compose
func Combine(extracts ...func(context.Context) []slog.Attr) func(context.Context) []slog.Attr {
	fs := make([]func(context.Context) []slog.Attr, 0, len(extracts))
	for _, f := range extracts {
		if f != nil {
			fs = append(fs, f)
		}
	}
	return func(ctx context.Context) []slog.Attr {
		var out []slog.Attr
		for _, f := range fs {
			out = append(out, f(ctx)...)
		}
		return out
	}
}
