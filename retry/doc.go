// Package retry calls a function again when it fails, waiting between attempts by a capped exponential backoff
// with jitter. Do and DoValue make at most WithAttempts attempts (default 3), wait the delay a Backoff gives
// (default FullJitter, base 100 ms, max 10 s) on a clock.Clock (WithClock), stop at once on an error marked
// Permanent or one WithRetryIf rejects (by default a context error and panics.ErrPanic), and return
// ErrExhausted wrapping the last error when every attempt failed. WithAttemptTimeout bounds each attempt;
// WithMaxElapsed caps the whole call without cutting an attempt in flight. Every call is bounded, by attempts,
// by elapsed time or by ctx: there is no retry-forever setting, at process start or anywhere else (a dependency
// that does not come up inside the bounds makes the process exit non-zero, so the orchestrator restarts it). The
// delays are calcbench algo/backoff (ALG-BACKOF-001..004), drawn from the WithRand source. Retry only work that
// is safe to repeat (idempotent): a call that failed after its side effect happened runs again.
//
// ID short form: RETRY (IDs are TB-RETRY-NNN).
//
// Exports:
//
//	TB-RETRY-001  Do                  calls fn until it succeeds, an error stops it, or the attempts run out
//	TB-RETRY-002  DoValue             Do for a function that returns a value
//	TB-RETRY-003  Permanent           marks an error as not retryable; errors.Is and errors.As still see it
//	TB-RETRY-004  ErrExhausted        every attempt failed; wraps the last attempt's error
//	TB-RETRY-005  ErrAttemptTimeout   the cause of an attempt's ctx when WithAttemptTimeout expires
//	TB-RETRY-006  Backoff             the delay between attempts: one calcbench backoff function, base and max
//	TB-RETRY-007  Exponential         a Backoff of min(max, base·2^k), no jitter
//	TB-RETRY-008  FullJitter          a Backoff of a uniform draw in [0, min(max, base·2^k))
//	TB-RETRY-009  EqualJitter         a Backoff of half of min(max, base·2^k) plus a draw in the other half
//	TB-RETRY-010  DecorrelatedJitter  a Backoff of min(max, a draw in [base, 3·previous delay))
//	TB-RETRY-011  Option              configures Do and DoValue
//	TB-RETRY-012  WithAttempts        sets the total number of attempts (default 3)
//	TB-RETRY-013  WithBackoff         sets the Backoff (default FullJitter(100ms, 10s))
//	TB-RETRY-014  WithAttemptTimeout  bounds each attempt; its ctx ends with ErrAttemptTimeout as the cause
//	TB-RETRY-015  WithRetryIf         decides which errors are retried (default: all but context errors and ErrPanic)
//	TB-RETRY-016  WithOnRetry         calls a function before each wait with the attempt, its error and the wait
//	TB-RETRY-017  WithClock           sets the clock the waits and attempt timeouts use (default clock.Real)
//	TB-RETRY-018  WithRand            sets the random source of the jitter draws (default math/rand/v2's top-level source)
//	TB-RETRY-019  WithMaxElapsed      caps the total time of a call: no wait or attempt starts past it, an attempt in flight is never cut (default none)
package retry
