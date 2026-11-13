package breaker

import "strconv"

// State is the state of a Breaker. The zero value is Closed.
//
// ID:          TB-BRK-004
// Pattern:     circuit breaker states (state machine)
// Authority:   Fowler, "CircuitBreaker", martinfowler.com, 2014 ("half open" state)
// Authority:   Microsoft, "Circuit Breaker pattern", Azure Architecture Center, "Solution" (Closed, Open, Half-Open)
// Tags:        breaker, state, resilience
type State int

// The states of a Breaker.
const (
	Closed   State = iota // calls run; consecutive failures are counted
	Open                  // calls are rejected with ErrOpen until OpenFor has passed
	HalfOpen              // up to the probe limit of calls run; their results close or reopen the breaker
)

// String returns "closed", "open" or "half-open", and "State(n)" for any other value.
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	}
	return "State(" + strconv.Itoa(int(s)) + ")"
}
