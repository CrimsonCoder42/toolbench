package logging_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"testing/slogtest"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/logging"
)

func TestRecorder_Slogtest(t *testing.T) {
	var rec *logging.Recorder
	slogtest.Run(t, func(*testing.T) slog.Handler {
		rec = new(logging.Recorder)
		return rec
	}, func(t *testing.T) map[string]any {
		var buf bytes.Buffer
		for _, r := range rec.Records() {
			if err := slog.NewJSONHandler(&buf, nil).Handle(context.Background(), r); err != nil {
				t.Fatal(err)
			}
		}
		return decodeOne(t, &buf)
	})
}

func TestRecorder_ZeroValueAndOrder(t *testing.T) {
	var rec logging.Recorder
	l := slog.New(&rec)
	for i := range 5 {
		l.Info(fmt.Sprint(i))
	}
	rs := rec.Records()
	if len(rs) != 5 {
		t.Fatalf("records = %d", len(rs))
	}
	for i, r := range rs {
		if r.Message != fmt.Sprint(i) {
			t.Fatalf("record %d out of order: %s", i, withheld(r.Message))
		}
	}
	rec.Reset()
	if len(rec.Records()) != 0 {
		t.Fatal("Reset did not empty")
	}
}

// lazy is a LogValuer that counts its calls; the Recorder must have resolved it once, at Handle.
type lazy struct{ n *int }

func (l lazy) LogValue() slog.Value { *l.n++; return slog.StringValue("resolved") }

func TestRecorder_FoldsChainAndResolves(t *testing.T) {
	var rec logging.Recorder
	n := 0
	l := slog.New(&rec).With("a", 1).WithGroup("g").With("b", lazy{&n}).WithGroup("h").WithGroup("")
	l.Info("m", slog.Group("c", slog.Any("d", lazy{&n})))
	l.Info("empty") // h is left empty and dropped; g keeps b
	if got := render(t, &rec); got != `{"level":"INFO","msg":"m","a":1,"g":{"b":"resolved","h":{"c":{"d":"resolved"}}}}`+"\n"+
		`level=INFO msg=m a=1 g.b=resolved g.h.c.d=resolved`+"\n"+
		`{"level":"INFO","msg":"empty","a":1,"g":{"b":"resolved"}}`+"\n"+
		`level=INFO msg=empty a=1 g.b=resolved`+"\n" {
		t.Fatalf("fold differs: %s", withheld(got))
	}
	if n != 2 {
		t.Fatalf("LogValue calls = %d, want 2 (once at WithAttrs, once at Handle)", n)
	}
}

func TestRecorder_RecordsAreCopies(t *testing.T) {
	var rec logging.Recorder
	slog.New(&rec).Info("m", "a", 1)
	rs := rec.Records()
	rs[0].AddAttrs(slog.Int("b", 2))
	if rec.Records()[0].NumAttrs() != 1 {
		t.Fatal("changing a returned record changed the Recorder")
	}
}

func TestRecorder_Nil(t *testing.T) {
	var rec *logging.Recorder
	if rec.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("nil Recorder enabled")
	}
	if err := rec.Handle(context.Background(), slog.NewRecord(fixed, slog.LevelInfo, "m", 0)); err != nil {
		t.Fatal(err)
	}
	rec.Reset()
	if rec.Records() != nil {
		t.Fatal("nil Recorder has records")
	}
}

// TestRecorder_ConcurrentUnderRace: 8 goroutines, each through its own With/WithGroup chain, 200 records
// each; every record is kept exactly once. Run with -race.
func TestRecorder_ConcurrentUnderRace(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	const workers, each = 8, 200
	var rec logging.Recorder
	base := slog.New(&rec)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			l := base.With("w", w).WithGroup("g")
			for i := range each {
				if ctx.Err() != nil {
					return
				}
				l.InfoContext(ctx, "m", "i", i)
				if i%50 == 0 {
					_ = rec.Records()
				}
			}
		})
	}
	wg.Wait()
	rs := rec.Records()
	if len(rs) != workers*each {
		t.Fatalf("records = %d, want %d", len(rs), workers*each)
	}
	seen := map[string]bool{}
	for _, r := range rs {
		var w, i int64
		r.Attrs(func(a slog.Attr) bool {
			switch a.Key {
			case "w":
				w = a.Value.Int64()
			case "g":
				i = a.Value.Group()[0].Value.Int64()
			}
			return true
		})
		k := fmt.Sprint(w, "/", i)
		if seen[k] {
			t.Fatalf("duplicate record %s", k)
		}
		seen[k] = true
	}
}

func TestRecorder_LimitCountsDropped(t *testing.T) {
	rec := logging.Recorder{Limit: 3}
	l := slog.New(&rec)
	for i := range 5 {
		l.Info(fmt.Sprint(i))
	}
	rs := rec.Records()
	if len(rs) != 3 || rs[2].Message != "2" || rec.Dropped() != 2 {
		t.Fatalf("kept %d, dropped %d", len(rs), rec.Dropped())
	}
	rec.Reset()
	if rec.Dropped() != 0 || len(rec.Records()) != 0 {
		t.Fatal("Reset did not clear")
	}
	var zero logging.Recorder
	for range 10001 {
		_ = zero.Handle(context.Background(), slog.NewRecord(fixed, slog.LevelInfo, "m", 0))
	}
	if len(zero.Records()) != 10000 || zero.Dropped() != 1 {
		t.Fatalf("default limit: kept %d, dropped %d", len(zero.Records()), zero.Dropped())
	}
}
