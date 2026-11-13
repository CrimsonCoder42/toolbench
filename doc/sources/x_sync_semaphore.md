# golang.org/x/sync/semaphore v0.25.0, pkg.go.dev and source

**Citation**: golang.org/x/sync/semaphore v0.25.0, pkg.go.dev and source.

**Opened**: https://pkg.go.dev/golang.org/x/sync/semaphore; https://raw.githubusercontent.com/golang/sync/master/semaphore/semaphore.go lines 48-57, 58, 68-73, 80-111, 123, 146-172 (opened 2026-11-13 by the semaphore builder).

**Used for**: The Weighted API and FIFO fairness (a front waiter that does not fit blocks later waiters); the panics and the block on n > size that D-015 replaces. compare/semaphore compares against it.

**Used by (IDs)**: TB-SEMA-001, TB-SEMA-002, TB-SEMA-003, TB-SEMA-004.
