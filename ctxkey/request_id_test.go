package ctxkey_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/CrimsonCoder42/toolbench/ctxkey"
	"github.com/CrimsonCoder42/toolbench/internal/leak"
)

func ExampleRequestID() {
	// A middleware stores the id; a logger (logging.FromKey) or a message publisher reads it back through the
	// same key, so every record of the request carries the same value.
	ctx, err := ctxkey.WithRequestID(context.Background(), "req-7f3a")
	fmt.Println(err)
	id, ok := ctxkey.RequestID.Value(ctx)
	fmt.Println(id, ok)
	fmt.Println(ctx)
	// Output:
	// <nil>
	// req-7f3a true
	// context.Background.WithValue(ctxkey.request-id, req-7f3a)
}

func ExampleNewRequestID() {
	id := ctxkey.NewRequestID()
	// The value is random and never printed: crypto/rand.Text gives 26 base32 characters today and may give
	// more in a future Go release, so only the floor and the validity are pinned.
	fmt.Println(len(id) >= 26, len(id) <= ctxkey.MaxRequestIDLen, ctxkey.ValidRequestID(id))
	// Output:
	// true true true
}

func ExampleValidRequestID() {
	fmt.Println(ctxkey.ValidRequestID("3f9c2e0a-1b7d-4e8f-9a6c-2d5b8e1f4a7c"))
	fmt.Println(ctxkey.ValidRequestID(""))
	fmt.Println(ctxkey.ValidRequestID("evil\r\nX-Injected: 1"))
	fmt.Println(ctxkey.ValidRequestID("two words"))
	// Output:
	// true
	// false
	// false
	// false
}

func ExampleWithRequestID() {
	base := context.Background()
	ctx, err := ctxkey.WithRequestID(base, "bad\nid")
	fmt.Println(errors.Is(err, ctxkey.ErrRequestID), ctx == base)
	ctx, err = ctxkey.WithRequestID(base, "good-id")
	id, _ := ctxkey.RequestIDFrom(ctx)
	fmt.Println(err, id)
	// Output:
	// true true
	// <nil> good-id
}

func ExampleRequestIDFrom() {
	id, ok := ctxkey.RequestIDFrom(context.Background())
	fmt.Printf("%q %v\n", id, ok)
	ctx, _ := ctxkey.WithRequestID(context.Background(), "abc")
	id, ok = ctxkey.RequestIDFrom(ctx)
	fmt.Printf("%q %v\n", id, ok)
	// Output:
	// "" false
	// "abc" true
}

func ExampleEnsureRequestID() {
	ctx, minted := ctxkey.EnsureRequestID(context.Background())
	again, kept := ctxkey.EnsureRequestID(ctx)
	fmt.Println(ctxkey.ValidRequestID(minted), kept == minted, again == ctx)
	// Output:
	// true true true
}

func ExampleMaxRequestIDLen() {
	fmt.Println(ctxkey.ValidRequestID(strings.Repeat("a", ctxkey.MaxRequestIDLen)))
	fmt.Println(ctxkey.ValidRequestID(strings.Repeat("a", ctxkey.MaxRequestIDLen+1)))
	// Output:
	// true
	// false
}

func ExampleErrRequestID() {
	_, err := ctxkey.WithRequestID(context.Background(), "")
	fmt.Println(errors.Is(err, ctxkey.ErrRequestID))
	fmt.Println(err)
	// Output:
	// true
	// ctxkey: invalid request id: empty
}

func TestNewRequestID_uniqueAndValid(t *testing.T) {
	leak.Verify(t)
	const n = 10_000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := ctxkey.NewRequestID()
		if !ctxkey.ValidRequestID(id) {
			t.Fatalf("id %d is not valid: %d bytes", i, len(id))
		}
		if len(id) < 26 {
			t.Fatalf("id %d is %d bytes, want >= 26 (crypto/rand.Text, >= 128 bits)", i, len(id))
		}
		for j := 0; j < len(id); j++ {
			c := id[j]
			if !(c >= 'A' && c <= 'Z') && !(c >= '2' && c <= '7') {
				t.Fatalf("id %d has byte %q outside the RFC 4648 base32 alphabet", i, c)
			}
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("id %d repeats an earlier id", i)
		}
		seen[id] = struct{}{}
	}
}

// TestNewRequestID_concurrent: no seed, no counter, no shared state; ids minted from many goroutines at once
// are all distinct and the goroutines all end.
func TestNewRequestID_concurrent(t *testing.T) {
	leak.Verify(t)
	const workers, each = 64, 100
	ids := make(chan string, workers*each) // every send has room: no goroutine blocks
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range each {
				ids <- ctxkey.NewRequestID()
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]struct{}, workers*each)
	for id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id across goroutines")
		}
		seen[id] = struct{}{}
	}
	if len(seen) != workers*each {
		t.Fatalf("got %d ids, want %d", len(seen), workers*each)
	}
}

func TestValidRequestID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"one byte", "a", true},
		{"128 bytes ok", strings.Repeat("x", 128), true},
		{"129 bytes too long", strings.Repeat("x", 129), false},
		{"space", "a b", false},
		{"leading space", " a", false},
		{"CR", "a\rb", false},
		{"LF", "a\nb", false},
		{"CRLF header injection", "id\r\nSet-Cookie: x", false},
		{"NUL", "a\x00b", false},
		{"tab", "a\tb", false},
		{"unicode", "идентификатор", false},
		{"unicode mixed", "abcé", false},
		{"DEL 0x7F", "a\x7f", false},
		{"0x20 just below range", "\x20", false},
		{"0x21 lowest ok", "!", true},
		{"0x7E highest ok", "~", true},
		{"byte 0x80", "a\x80", false},
		{"byte 0xFF", "\xff", false},
		{"UUID", "3f9c2e0a-1b7d-4e8f-9a6c-2d5b8e1f4a7c", true},
		{"traceparent", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", true},
		{"chi style host/counter", "myhost.local/8kAbCdEf-000123", true},
		{"minted", ctxkey.NewRequestID(), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ctxkey.ValidRequestID(c.in); got != c.want {
				t.Errorf("ValidRequestID(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestWithRequestID_invalidLeavesCtxUnchanged is the M4 "request id" defect: GO-CQRS-Microservices
// internal/pkg/http/echo/middleware/correlation_id_middleware.go:9-25 reuses an inbound header unvalidated, so
// a 10 KB id or one carrying CR/LF reaches the context, the log and the response. Here such an id is rejected:
// the returned context is the very same value, nothing is stored, and the error matches ErrRequestID.
func TestWithRequestID_invalidLeavesCtxUnchanged(t *testing.T) {
	leak.Verify(t)
	base, _ := ctxkey.WithRequestID(context.Background(), "existing")
	cases := []struct {
		name string
		id   string
	}{
		{"10 KB", strings.Repeat("A", 10*1024)},
		{"CRLF", "abc\r\nX-Forged: yes"},
		{"LF only", "abc\nforged"},
		{"empty", ""},
		{"NUL", "abc\x00"},
		{"129 bytes", strings.Repeat("A", ctxkey.MaxRequestIDLen+1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ctxkey.WithRequestID(base, c.id)
			if !errors.Is(err, ctxkey.ErrRequestID) {
				t.Fatalf("err = %v, want ErrRequestID", err)
			}
			if got != base {
				t.Errorf("ctx changed: got %v, want the same context value", got)
			}
			if id, _ := ctxkey.RequestIDFrom(got); id != "existing" {
				t.Errorf("stored id = %q, want the previous id untouched", id)
			}
		})
	}
}

// TestWithRequestID_errorNeverEchoesID: the error is the first thing a caller logs, so it must not carry the
// attacker's bytes (log forging through the error path would undo the validator).
func TestWithRequestID_errorNeverEchoesID(t *testing.T) {
	for _, id := range []string{"evil\r\nX-Forged: yes", strings.Repeat("Z", 10*1024), "\x00bad"} {
		_, err := ctxkey.WithRequestID(context.Background(), id)
		if err == nil {
			t.Fatalf("WithRequestID(%q) accepted an invalid id", id)
		}
		msg := err.Error()
		if strings.Contains(msg, "\r") || strings.Contains(msg, "\n") || strings.Contains(msg, "\x00") {
			t.Errorf("error message carries a control byte: %q", msg)
		}
		if strings.Contains(msg, "X-Forged") || strings.Contains(msg, "ZZZZ") || strings.Contains(msg, "bad") {
			t.Errorf("error message echoes the rejected id: %q", msg)
		}
		if !strings.HasPrefix(msg, ctxkey.ErrRequestID.Error()) {
			t.Errorf("error message %q does not start with the sentinel's text", msg)
		}
	}
}

func TestWithRequestID_nilCtx(t *testing.T) {
	ctx, err := ctxkey.WithRequestID(nil, "ok") //nolint:staticcheck // nil ctx is the input under test
	if err != nil {
		t.Fatalf("valid id on nil ctx: err = %v", err)
	}
	if ctx == nil {
		t.Fatal("returned a nil context")
	}
	if id, ok := ctxkey.RequestIDFrom(ctx); !ok || id != "ok" {
		t.Errorf("got (%q, %v), want (ok, true)", id, ok)
	}
	if ctx.Done() != nil {
		t.Error("expected a Background-derived context (never cancelled)")
	}

	ctx, err = ctxkey.WithRequestID(nil, "") //nolint:staticcheck // nil ctx is the input under test
	if !errors.Is(err, ctxkey.ErrRequestID) {
		t.Fatalf("invalid id on nil ctx: err = %v, want ErrRequestID", err)
	}
	if ctx == nil {
		t.Fatal("invalid id on nil ctx returned a nil context; want context.Background")
	}
	if _, ok := ctxkey.RequestIDFrom(ctx); ok {
		t.Error("invalid id was stored")
	}
}

func TestEnsureRequestID_keepsExistingAndMintsMissing(t *testing.T) {
	leak.Verify(t)
	// Existing, valid: same ctx value, same id.
	base, _ := ctxkey.WithRequestID(context.Background(), "keep-me")
	ctx, id := ctxkey.EnsureRequestID(base)
	if id != "keep-me" {
		t.Errorf("kept id = %q, want keep-me", id)
	}
	if ctx != base {
		t.Error("Ensure on a ctx with a valid id must return the same ctx value")
	}

	// Missing: minted, valid, stored, and stable on the next call.
	ctx, minted := ctxkey.EnsureRequestID(context.Background())
	if !ctxkey.ValidRequestID(minted) {
		t.Fatalf("minted id %q is not valid", minted)
	}
	if got, ok := ctxkey.RequestIDFrom(ctx); !ok || got != minted {
		t.Errorf("stored (%q, %v), want (%q, true)", got, ok, minted)
	}
	if again, id2 := ctxkey.EnsureRequestID(ctx); id2 != minted || again != ctx {
		t.Errorf("second Ensure: (%v, %q), want the same ctx and %q", again == ctx, id2, minted)
	}
	if _, other := ctxkey.EnsureRequestID(context.Background()); other == minted {
		t.Error("two Ensure calls on fresh contexts minted the same id")
	}

	// Stored but invalid (a consumer bypassed WithRequestID): replaced, not forwarded.
	bad := ctxkey.RequestID.With(context.Background(), "forged\r\nX: y")
	ctx, replaced := ctxkey.EnsureRequestID(bad)
	if !ctxkey.ValidRequestID(replaced) || replaced == "forged\r\nX: y" {
		t.Errorf("invalid stored id was not replaced: %q", replaced)
	}
	if got, _ := ctxkey.RequestIDFrom(ctx); got != replaced {
		t.Errorf("child holds %q, want the replacement %q", got, replaced)
	}
	if got, _ := ctxkey.RequestIDFrom(bad); got != "forged\r\nX: y" {
		t.Error("Ensure changed the parent context")
	}

	// Nil ctx: Background, minted.
	ctx, id = ctxkey.EnsureRequestID(nil) //nolint:staticcheck // nil ctx is the input under test
	if ctx == nil || !ctxkey.ValidRequestID(id) {
		t.Errorf("nil ctx: got (%v, %q)", ctx, id)
	}
}

func TestRequestIDFrom_nilCtx(t *testing.T) {
	id, ok := ctxkey.RequestIDFrom(nil) //nolint:staticcheck // nil ctx is the input under test
	if ok || id != "" {
		t.Errorf("got (%q, %v), want (\"\", false)", id, ok)
	}
	id, ok = ctxkey.RequestIDFrom(context.TODO())
	if ok || id != "" {
		t.Errorf("TODO: got (%q, %v), want (\"\", false)", id, ok)
	}
}

func BenchmarkNewRequestID(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if id := ctxkey.NewRequestID(); len(id) == 0 {
			b.Fatal("empty id")
		}
	}
}

func BenchmarkEnsureRequestID(b *testing.B) {
	b.Run("hit", func(b *testing.B) {
		ctx, _ := ctxkey.EnsureRequestID(context.Background())
		b.ReportAllocs()
		for b.Loop() {
			if _, id := ctxkey.EnsureRequestID(ctx); len(id) == 0 {
				b.Fatal("empty id")
			}
		}
	})
	b.Run("miss", func(b *testing.B) {
		base := context.Background()
		b.ReportAllocs()
		for b.Loop() {
			if _, id := ctxkey.EnsureRequestID(base); len(id) == 0 {
				b.Fatal("empty id")
			}
		}
	})
}

func BenchmarkValidRequestID(b *testing.B) {
	id := ctxkey.NewRequestID()
	b.ReportAllocs()
	for b.Loop() {
		if !ctxkey.ValidRequestID(id) {
			b.Fatal("invalid")
		}
	}
}
