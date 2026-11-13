package logging

import (
	"context"
	"log/slog"
	"sync"
)

// Recorder is a slog.Handler for tests that keeps every record it is given. Each stored record has every value
// resolved (LogValuer, recursively in groups) and the WithAttrs/WithGroup chain of the handler that took it
// folded in (With attributes first, then the record's attributes inside the groups that were open), with empty
// attributes and groups left empty dropped, so a test sees what an output handler would print. Enabled is true at every level.
// Handlers made by WithAttrs and WithGroup write into the same Recorder. It is bounded: it keeps at most Limit
// records (0 or less: 10000) and counts every later one in Dropped instead of storing it; set Limit before
// first use. All methods are safe for concurrent use. The zero value is ready to use (var r logging.Recorder; slog.New(&r)); a Recorder must not be copied
// after first use. A nil *Recorder is never enabled and stores nothing.
//
// ID:          TB-LOG-010
// Pattern:     test double (test spy: a recording handler)
// Authority:   Fowler, "TestDouble", martinfowler.com, 2006 (after Meszaros, xUnit Test Patterns, 2007)
// Authority:   pkg.go.dev log/slog, "type Handler" (Handle rules) and pkg.go.dev testing/slogtest
// Tags:        logging, slog, testing, recorder, handler
type Recorder struct {
	// Limit is the most records kept; 0 or less means defaultRecorderLimit (10000). Set it before first use.
	Limit int

	mu      sync.Mutex    // guards records and dropped
	records []slog.Record // in arrival order, at most the limit
	dropped int           // records refused because the limit was reached
}

// defaultRecorderLimit is the Recorder's cap when Limit is 0 or less.
const defaultRecorderLimit = 10000

// Enabled reports true for every level (false for a nil Recorder).
func (r *Recorder) Enabled(context.Context, slog.Level) bool { return r != nil }

// Handle stores a copy of rec with its values resolved. It never fails.
func (r *Recorder) Handle(_ context.Context, rec slog.Record) error {
	r.add(nil, rec)
	return nil
}

// WithAttrs returns a handler that stores into r with attrs added before each record's attributes.
func (r *Recorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	return (&recorderView{root: r}).WithAttrs(attrs)
}

// WithGroup returns a handler that stores into r with each record's attributes inside group name;
// WithGroup("") returns r.
func (r *Recorder) WithGroup(name string) slog.Handler {
	if name == "" {
		return r
	}
	return (&recorderView{root: r}).WithGroup(name)
}

// Records returns clones of the stored records in arrival order; changing them does not change r.
func (r *Recorder) Records() []slog.Record {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]slog.Record, len(r.records))
	for i, rec := range r.records {
		out[i] = rec.Clone()
	}
	return out
}

// Dropped returns how many records were refused because the Recorder held Limit records.
func (r *Recorder) Dropped() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

// Reset removes every stored record and sets Dropped back to 0.
func (r *Recorder) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.records, r.dropped = nil, 0
	r.mu.Unlock()
}

// add folds chain into rec, resolves its values and stores it.
func (r *Recorder) add(chain []step, rec slog.Record) {
	if r == nil {
		return
	}
	cur := make([]slog.Attr, 0, rec.NumAttrs())
	rec.Attrs(func(a slog.Attr) bool {
		cur = append(cur, a)
		return true
	})
	cur = cleanAttrs(cur)
	for i := len(chain) - 1; i >= 0; i-- {
		s := chain[i]
		if s.group != "" {
			if len(cur) == 0 {
				continue
			}
			cur = []slog.Attr{{Key: s.group, Value: slog.GroupValue(cur...)}}
			continue
		}
		cur = append(append([]slog.Attr(nil), s.attrs...), cur...)
	}
	nr := slog.NewRecord(rec.Time, rec.Level, rec.Message, rec.PC)
	nr.AddAttrs(cur...)
	limit := r.Limit
	if limit <= 0 {
		limit = defaultRecorderLimit
	}
	r.mu.Lock()
	if len(r.records) < limit {
		r.records = append(r.records, nr)
	} else {
		r.dropped++
	}
	r.mu.Unlock()
}

// cleanAttrs returns as with every value resolved (recursively in groups), empty attrs (slog.Attr{}) dropped
// and groups left with no attrs dropped: the Handler rules of log/slog, applied once at capture.
func cleanAttrs(as []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, 0, len(as))
	for _, a := range as {
		if a.Equal(slog.Attr{}) {
			continue
		}
		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			g := cleanAttrs(a.Value.Group())
			if len(g) == 0 {
				continue
			}
			a.Value = slog.GroupValue(g...)
		}
		out = append(out, a)
	}
	return out
}

// recorderView is a Recorder seen through a WithAttrs/WithGroup chain. Immutable.
type recorderView struct {
	root  *Recorder
	chain []step
}

func (v *recorderView) Enabled(ctx context.Context, l slog.Level) bool { return v.root.Enabled(ctx, l) }

func (v *recorderView) Handle(_ context.Context, rec slog.Record) error {
	v.root.add(v.chain, rec)
	return nil
}

func (v *recorderView) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return v
	}
	res := cleanAttrs(attrs)
	if len(res) == 0 {
		return v
	}
	return v.with(step{attrs: res})
}

func (v *recorderView) WithGroup(name string) slog.Handler {
	if name == "" {
		return v
	}
	return v.with(step{group: name})
}

func (v *recorderView) with(s step) *recorderView {
	chain := make([]step, len(v.chain), len(v.chain)+1)
	copy(chain, v.chain)
	return &recorderView{root: v.root, chain: append(chain, s)}
}
