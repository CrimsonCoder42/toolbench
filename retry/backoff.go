package retry

import (
	"time"

	"github.com/CrimsonCoder42/calcbench/algo/backoff"
)

// Default backoff bounds: the base of the first wait and the cap of every wait.
const (
	defaultBase = 100 * time.Millisecond
	defaultMax  = 10 * time.Second
)

type kind uint8

const (
	kindFull kind = iota // the zero value: FullJitter
	kindExponential
	kindEqual
	kindDecorrelated
)

// Backoff is the delay between attempts: one of Brooker's capped exponential schedules (Exponential,
// FullJitter, EqualJitter, DecorrelatedJitter) with its base and cap. Every delay is computed by calcbench
// algo/backoff; nothing is computed here. Create one with those functions. The zero value is FullJitter with
// base 100 ms and max 10 s. A base <= 0 is 100 ms; a max <= 0 is the larger of 10 s and base; a max below base
// is base (so Exponential(d, d) is a constant delay d). A Backoff is a value and safe to copy and share.
//
// ID:          TB-RETRY-006
// Pattern:     capped exponential backoff with jitter
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Jitter"
// Tags:        retry, backoff, jitter, duration
type Backoff struct {
	kind      kind
	base, max time.Duration
}

func newBackoff(k kind, base, max time.Duration) Backoff {
	if base <= 0 {
		base = defaultBase
	}
	switch {
	case max <= 0:
		max = defaultMax
		if max < base {
			max = base
		}
	case max < base:
		max = base
	}
	return Backoff{kind: k, base: base, max: max}
}

// Exponential returns a Backoff with no jitter: the wait before retry k (k = 0 for the first retry) is
// min(max, base·2^k), saturating. Exponential(d, d) waits d every time.
//
// ID:          TB-RETRY-007
// Pattern:     capped exponential backoff
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Backoff"
// Authority:   gRPC, "Connection Backoff Protocol", grpc/grpc doc/connection-backoff.md, "Proposed Backoff Algorithm"
// Math:        ALG-BACKOF-001
// Tags:        retry, backoff, exponential, duration
func Exponential(base, max time.Duration) Backoff { return newBackoff(kindExponential, base, max) }

// FullJitter returns a Backoff whose wait before retry k is a uniform draw in [0, min(max, base·2^k)). It is
// the default (base 100 ms, max 10 s): Brooker finds it does the least work of the schedules he compares.
//
// ID:          TB-RETRY-008
// Pattern:     full jitter
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Jitter" (Full Jitter)
// Math:        ALG-BACKOF-002
// Tags:        retry, backoff, jitter, full-jitter, duration
func FullJitter(base, max time.Duration) Backoff { return newBackoff(kindFull, base, max) }

// EqualJitter returns a Backoff whose wait before retry k keeps half of s = min(max, base·2^k) and adds a
// uniform draw in [0, s/2): a value in [⌈s/2⌉, s].
//
// ID:          TB-RETRY-009
// Pattern:     equal jitter
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Jitter" (Equal Jitter)
// Math:        ALG-BACKOF-003
// Tags:        retry, backoff, jitter, equal-jitter, duration
func EqualJitter(base, max time.Duration) Backoff { return newBackoff(kindEqual, base, max) }

// DecorrelatedJitter returns a Backoff whose wait is min(max, a uniform draw in [base, 3·prev)), where prev is
// the previous wait of the same Do call (base before the first retry).
//
// ID:          TB-RETRY-010
// Pattern:     decorrelated jitter
// Authority:   Brooker, "Exponential Backoff And Jitter", AWS Architecture Blog, 2015, "Adding Jitter" (Decorrelated Jitter)
// Math:        ALG-BACKOF-004
// Tags:        retry, backoff, jitter, decorrelated-jitter, duration
func DecorrelatedJitter(base, max time.Duration) Backoff {
	return newBackoff(kindDecorrelated, base, max)
}

// normalized gives the zero Backoff its defaults.
func (b Backoff) normalized() Backoff { return newBackoff(b.kind, b.base, b.max) }

// next returns the wait before retry k (0-based), given the previous wait prev (base before the first retry)
// and a draw function called only when the schedule is jittered. b must be normalized, so calcbench's
// argument errors cannot occur; one is returned unchanged if it does.
func (b Backoff) next(k int, prev time.Duration, draw func() float64) (time.Duration, error) {
	switch b.kind {
	case kindExponential:
		return backoff.Exponential(k, b.base, b.max)
	case kindEqual:
		return backoff.EqualJitter(k, b.base, b.max, draw())
	case kindDecorrelated:
		return backoff.DecorrelatedJitter(prev, b.base, b.max, draw())
	default:
		return backoff.FullJitter(k, b.base, b.max, draw())
	}
}
