package pubsub_test

import (
	"context"
	"errors"
	"testing"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/pubsub"
)

func TestMessage_header(t *testing.T) {
	leak.Verify(t)
	var zero pubsub.Message[int]
	if v, ok := zero.Header(pubsub.HeaderRequestID); ok || v != "" {
		t.Fatalf("zero value: %q %v", v, ok)
	}
	m := pubsub.Message[int]{Headers: map[string]string{pubsub.HeaderRequestID: "r1"}, Body: 7}
	if v, ok := m.Header(pubsub.HeaderRequestID); !ok || v != "r1" {
		t.Fatalf("present: %q %v", v, ok)
	}
	if v, ok := m.Header(pubsub.HeaderCorrelationID); ok || v != "" {
		t.Fatalf("missing: %q %v", v, ok)
	}
	if m.Body != 7 {
		t.Fatal("body")
	}
}

func TestMessage_withHeaderIsCopyOnWrite(t *testing.T) {
	leak.Verify(t)
	var zero pubsub.Message[string]
	one := zero.WithHeader("k", "v")
	if zero.Headers != nil || len(one.Headers) != 1 || one.Headers["k"] != "v" {
		t.Fatalf("from nil: %v -> %v", zero.Headers, one.Headers)
	}
	shared := map[string]string{pubsub.HeaderCorrelationID: "c1"}
	m := pubsub.Message[string]{Headers: shared, Body: "b"}
	withReq := m.WithHeader(pubsub.HeaderRequestID, "r2")
	replaced := m.WithHeader(pubsub.HeaderCorrelationID, "c3")
	if len(shared) != 1 || shared[pubsub.HeaderCorrelationID] != "c1" {
		t.Fatalf("the shared map changed: %v", shared)
	}
	if len(withReq.Headers) != 2 || withReq.Headers[pubsub.HeaderRequestID] != "r2" || withReq.Headers[pubsub.HeaderCorrelationID] != "c1" || withReq.Body != "b" {
		t.Fatalf("withReq %+v", withReq)
	}
	if len(replaced.Headers) != 1 || replaced.Headers[pubsub.HeaderCorrelationID] != "c3" {
		t.Fatalf("replaced %+v", replaced)
	}
	withReq.Headers["x"] = "y" // writing the copy never reaches the source
	if _, ok := m.Header("x"); ok {
		t.Fatal("WithHeader shared its map with the source")
	}
}

// The envelope is delivered by value with its map shared read-only: a handler that calls WithHeader (and keeps
// the copy to itself) changes nothing for the other subscriber of the same message, nor for the publisher.
func TestMessage_withHeaderInOneHandlerDoesNotAffectAnother(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	type M = pubsub.Message[string]
	hook, ready := counter[M](2)
	bus := pubsub.NewBus[M](hook)
	gotA, gotB := make(chan M, 1), make(chan M, 1)
	a := subscribe(bus, ctx, "t", func(_ context.Context, m M) error {
		gotA <- m.WithHeader("handled-by", "a")
		return errStop
	})
	b := subscribe(bus, ctx, "t", func(_ context.Context, m M) error {
		gotB <- m
		return errStop
	})
	recv(t, ctx, ready)
	orig := M{Headers: map[string]string{pubsub.HeaderCorrelationID: "c1"}, Body: "x"}
	if err := bus.Publish(ctx, "t", orig); err != nil {
		t.Fatal(err)
	}
	for _, d := range []<-chan error{a, b} {
		if err := recv(t, ctx, d); !errors.Is(err, errStop) {
			t.Fatal(err)
		}
	}
	ma, mb := recv(t, ctx, gotA), recv(t, ctx, gotB)
	if v, ok := ma.Header("handled-by"); !ok || v != "a" {
		t.Fatalf("a's copy: %v", ma.Headers)
	}
	if _, ok := mb.Header("handled-by"); ok || len(mb.Headers) != 1 {
		t.Fatalf("b saw a's header: %v", mb.Headers)
	}
	if _, ok := orig.Header("handled-by"); ok || len(orig.Headers) != 1 {
		t.Fatalf("the publisher's map changed: %v", orig.Headers)
	}
}

// Kafka-GO-Real-Time-Feed producer/main.go:93-94 sets a correlation id on the produced message and
// consumer/main.go:68-70 copies the consumed message's id onto every message the handler produces. Ours: the
// request id set by the publisher reaches the handler, which republishes on another topic with that id as the
// correlation id; the original envelope is untouched.
func TestMessage_correlationPropagatesThroughHandler(t *testing.T) {
	leak.Verify(t)
	ctx := bounded(t)
	type M = pubsub.Message[string]
	hook, ready := counter[M](1)
	bus := pubsub.NewBus[M](hook)
	got := make(chan M, 1)
	invoices := subscribe(bus, ctx, "invoices", func(_ context.Context, m M) error {
		got <- m
		return errStop
	})
	orders := subscribe(bus, ctx, "orders", func(hctx context.Context, m M) error {
		reqID, ok := m.Header(pubsub.HeaderRequestID)
		if !ok {
			return errors.New("no request id on the order")
		}
		invoice := m.WithHeader(pubsub.HeaderCorrelationID, reqID)
		invoice.Body = "invoice for " + m.Body
		if err := bus.Publish(hctx, "invoices", invoice); err != nil {
			return err
		}
		return errStop
	})
	recv(t, ctx, ready)
	recv(t, ctx, ready)
	order := M{Body: "latte"}.WithHeader(pubsub.HeaderRequestID, "req-1")
	if err := bus.Publish(ctx, "orders", order); err != nil {
		t.Fatal(err)
	}
	for _, d := range []<-chan error{orders, invoices} {
		if err := recv(t, ctx, d); !errors.Is(err, errStop) {
			t.Fatal(err)
		}
	}
	inv := recv(t, ctx, got)
	if c, ok := inv.Header(pubsub.HeaderCorrelationID); !ok || c != "req-1" {
		t.Fatalf("correlation id %q %v", c, ok)
	}
	if r, ok := inv.Header(pubsub.HeaderRequestID); !ok || r != "req-1" || inv.Body != "invoice for latte" {
		t.Fatalf("invoice %+v", inv)
	}
	if _, ok := order.Header(pubsub.HeaderCorrelationID); ok || len(order.Headers) != 1 {
		t.Fatalf("the order's headers changed: %v", order.Headers)
	}
}
