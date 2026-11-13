// Package ratelimit limits how often events happen. Limiter is a token bucket shared by goroutines with
// golang.org/x/time/rate's method set (Allow, AllowN, Reserve, ReserveN, Wait, WaitN, Limit, Burst) on an
// exact integer rate (Limit: Events per Per). Keyed holds one bucket per key in a bounded table with idle
// eviction, for per-client limits; its Allow returns the retry-after an HTTP 429 needs. The bucket
// arithmetic is calcbench ALG-RATELM-001 TokenBucket (exact integer refill) and Reservation.Cancel gives
// tokens back with ALG-RATELM-005 TokenBucket.Restore by x/time/rate's CancelAt rule; Keyed's table is
// ALG-LIST-015 LRUCache with ALG-LIST-018 Oldest for idle eviction; each is guarded by one mutex per
// limiter. Every reading of time and every wait uses a clock.Clock (WithClock), so tests use clock.Fake. No
// goroutine is started.
//
// ID short form: RATE (IDs are TB-RATE-NNN).
//
// Exports:
//
//	TB-RATE-001  Limit        a rate: Events events every Per, exact integers
//	TB-RATE-002  Every        the Limit of one event every interval
//	TB-RATE-003  Limiter      a token bucket shared by goroutines: Allow, Reserve, Wait (x/time/rate shape)
//	TB-RATE-004  Reservation  tokens taken now for events after a delay; Cancel gives them back
//	TB-RATE-005  Keyed        one token bucket per key in a bounded table with idle eviction
//	TB-RATE-006  Option       configures NewLimiter and NewKeyed
//	TB-RATE-007  WithClock    sets the clock the limiter reads and waits on (default clock.Real)
//	TB-RATE-008  WithMaxKeys  bounds the keys a Keyed limiter holds (default 10 000)
//	TB-RATE-009  WithIdleTTL  sets how long an unused key is kept (default the full-refill time)
//	TB-RATE-010  ErrLimit     the sentinel for a Limit that is not valid
//	TB-RATE-011  ErrBurst     the sentinel for burst < 1, or n greater than the burst
//	TB-RATE-012  ErrKeys      the sentinel for a new key when the key table is full and nothing is idle
//	TB-RATE-013  DefaultMaxKeys  the key bound of Keyed when WithMaxKeys is not given: 10 000
//	TB-RATE-014  InfDuration     the Delay of a Reservation that is not OK
//	TB-RATE-015  NewLimiter      a full Limiter for a rate and burst; ErrLimit or ErrBurst when invalid
//	TB-RATE-016  NewKeyed        an empty Keyed limiter for a rate and burst; ErrLimit or ErrBurst when invalid
package ratelimit
