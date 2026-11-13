# Effective Go, go.dev

**Citation**: Effective Go, go.dev.

**Opened**: https://go.dev/doc/effective_go#recover (opened 2026-11-13 by the panics builder).

**Used for**: Section "Recover": a server goroutine converts a panic in a work item into a logged error without killing the process; the rule that a worker panic becomes an error (go-concurrency.md rule 6).

**Also**: section "Concurrency", "Share by communicating" (opened 2026-11-13 by the guard builder): the confinement rationale for Guarded and Confine.

**Used by (IDs)**: TB-PANIC-003, TB-PANIC-004, TB-GUARD-001, TB-GUARD-002.
