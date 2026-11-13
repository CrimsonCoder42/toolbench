# Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014

**Citation**: Ajmani, "Go Concurrency Patterns: Pipelines and cancellation", The Go Blog, 2014.

**Opened**: https://go.dev/blog/pipelines (opened 2026-11-13 by the flow builder).

**Used for**: Sections "What is a pipeline?", "Squaring numbers", "Fan-out, fan-in", "Explicit cancellation", "Bounded parallelism": the stage shape (the creator closes its outbound channel), fan-out/fan-in, cancellation by a done channel (here ctx). Align run 2026-11-13: "Stopping short" for the eviction policy.

**Also** (opened 2026-11-13 by the pool and parallel builders): "Bounded parallelism", the fixed worker count behind pool and parallel.

**Used by (IDs)**: TB-FLOW-001, TB-FLOW-003, TB-FLOW-004, TB-FLOW-005, TB-FLOW-006, TB-POOL-001, TB-POOL-002, TB-POOL-004, TB-PAR-001, TB-PAR-002, TB-PAR-003, TB-BATCH-002, TB-PUBSUB-006, TB-PUBSUB-015.
