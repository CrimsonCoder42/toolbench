# The Go Memory Model, go.dev

**Citation**: The Go Memory Model, go.dev.

**Opened**: https://go.dev/ref/mem (opened 2026-11-13 by the guard builder).

**Used for**: Sections "Synchronization", "Locks": an Unlock happens before the next Lock returns, the guarantee Guarded relies on (go-concurrency.md rule 8). Align run 2026-11-13: the per-key lock of guard.Keyed ("Locks", "Channel communication").

**Used by (IDs)**: TB-GUARD-001, TB-GUARD-003, TB-GUARD-004.
