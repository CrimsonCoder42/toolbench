package ratelimit

import "time"

// Limit is a rate: Events events every Per. It is exact integers, not x/time/rate's float64 events per
// second, so a limiter never drifts and Every(d) is exact for every d. rate.Limit(r) for a whole r is
// Limit{Events: r, Per: time.Second}; rate.Every(d) is Every(d). A Limit is valid when Events >= 1 and
// Per > 0; NewLimiter and NewKeyed return ErrLimit otherwise (there is no Inf and no zero rate).
//
// ID:          TB-RATE-001
// Pattern:     token-bucket rate (CIR as tokens per interval)
// Authority:   pkg.go.dev golang.org/x/time/rate, "type Limit"
// Authority:   Heinanen and Guérin, "A Single Rate Three Color Marker", RFC 2697, IETF, 1999, §3 Metering
// Tags:        rate-limiting, rate, token-bucket
type Limit struct {
	Events int64         // tokens added every Per; at least 1
	Per    time.Duration // the interval; positive
}

// Every returns the Limit of one event every interval, as x/time/rate's Every. An interval <= 0 gives an
// invalid Limit (NewLimiter returns ErrLimit) where x/time/rate gives Inf.
//
// ID:          TB-RATE-002
// Pattern:     token-bucket rate (minimum time between events)
// Authority:   pkg.go.dev golang.org/x/time/rate, "func Every"
// Tags:        rate-limiting, rate, interval
func Every(interval time.Duration) Limit {
	return Limit{Events: 1, Per: interval}
}
