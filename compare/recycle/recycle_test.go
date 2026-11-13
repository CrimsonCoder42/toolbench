// Package recycle_test compares toolbench/recycle.Pool with github.com/valyala/bytebufferpool (written against
// v1.0.0) where their shapes meet, buffer reuse with reset, and with sync.Pool itself: the same script of Get,
// write and Put gives an empty buffer from the next Get in all three.
//
// Known differences, by design:
//   - bytebufferpool pools only its own *ByteBuffer and always resets on Put; recycle.Pool is generic and
//     resets only through WithReset.
//   - bytebufferpool drops oversized buffers by a size it calibrates from usage (a percentile of recent
//     sizes); recycle.Pool drops what the caller's WithKeep rejects, a fixed rule the caller states. The
//     calibration is not compared (it needs tens of thousands of Puts and is not deterministic in a test).
//   - sync.Pool returns any (a type assertion at every Get) and returns nil from an empty pool without New;
//     recycle.Pool returns T, and the zero T without a constructor.
package recycle_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/CrimsonCoder42/toolbench/recycle"
	"github.com/valyala/bytebufferpool"
)

// TestRecycle_resetOnPut_matchesByteBufferPool: write, Put, Get gives an empty buffer in both.
func TestRecycle_resetOnPut_matchesByteBufferPool(t *testing.T) {
	var theirs bytebufferpool.Pool
	tb := theirs.Get()
	tb.WriteString("hello")
	theirs.Put(tb)
	if n := theirs.Get().Len(); n != 0 {
		t.Fatalf("bytebufferpool: Len after Put/Get = %d, want 0", n)
	}

	ours := recycle.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		recycle.WithReset(func(b *bytes.Buffer) *bytes.Buffer { b.Reset(); return b }),
	)
	ob := ours.Get()
	ob.WriteString("hello")
	ours.Put(ob)
	if n := ours.Get().Len(); n != 0 {
		t.Fatalf("recycle: Len after Put/Get = %d, want 0", n)
	}
}

// TestRecycle_keepDropsOversized: recycle.WithKeep drops a buffer above the cap at once (bytebufferpool would
// keep it until its calibration notices; golang/go#23199).
func TestRecycle_keepDropsOversized(t *testing.T) {
	const maxCap = 4 << 10
	ours := recycle.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		recycle.WithKeep(func(b *bytes.Buffer) bool { return b.Cap() <= maxCap }),
	)
	big := ours.Get()
	big.Grow(1 << 20)
	ours.Put(big)
	if got := ours.Get(); got == big || got.Cap() > maxCap {
		t.Fatalf("recycle: the oversized buffer was pooled (cap %d)", got.Cap())
	}
}

// TestRecycle_emptyGet_matchesSyncPool: an empty pool with no constructor gives nil from sync.Pool and the
// zero T from recycle.Pool (nil for a pointer type).
func TestRecycle_emptyGet_matchesSyncPool(t *testing.T) {
	var theirs sync.Pool
	if got := theirs.Get(); got != nil {
		t.Fatalf("sync.Pool: got %v, want nil", got)
	}
	var ours recycle.Pool[*bytes.Buffer]
	if got := ours.Get(); got != nil {
		t.Fatalf("recycle: got %v, want nil", got)
	}
}

func BenchmarkByteBufferPool_GetPut(b *testing.B) {
	var p bytebufferpool.Pool
	b.ReportAllocs()
	for b.Loop() {
		buf := p.Get()
		buf.WriteString("x")
		p.Put(buf)
	}
}

func BenchmarkRecycle_GetPut(b *testing.B) {
	p := recycle.New(
		func() *bytes.Buffer { return new(bytes.Buffer) },
		recycle.WithReset(func(buf *bytes.Buffer) *bytes.Buffer { buf.Reset(); return buf }),
	)
	b.ReportAllocs()
	for b.Loop() {
		buf := p.Get()
		buf.WriteString("x")
		p.Put(buf)
	}
}
