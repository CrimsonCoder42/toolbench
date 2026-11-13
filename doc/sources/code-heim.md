# code-heim (github.com/code-heim)

**What it is**: a GitHub organisation of short Go tutorial repositories, one technique per repository, numbered
in the order they were published (go_2 ... go_85, plus socket_io_emitter); numbers 1, 16, 22, 29 and 84 do not
exist. 81 repositories on 2026-11-13, last pushed between 2023-11-27 and 2025-04-23.

**What it is used for**: a READING LIST ONLY, and toolbench's true north for ORGANISING: which patterns matter
to a working Go service and in what order (decision D-004). It is never copied and never cited. Every pattern it
shows is re-researched against today's Go (go1.27) and cited to its authority (`.claude/rules/sources.md`). No
toolbench file adapts code-heim code, so no file carries a code-heim attribution notice; if one ever does, its
header carries the notice of that repository's licence.

**Licences** (GitHub API, 2026-11-13; the full list is `doc/ladder_2026-11-13/README.md`):
- Apache-2.0: 78 repositories.
- MIT: go_7_go_decorator, go_8_redis_lock.
- None declared: go_55_grpc (no LICENSE file; all rights reserved by default, so nothing from it may be adapted).

**Local copy**: `_reference/code-heim/<name>`, `git clone --depth 1`, gitignored, read-only: never edited, never
built, never run, never `go mod download`.

**Read for**: `doc/ladder_2026-11-13/` (R1-R4 per repository, merged in LADDER.md).

**Used by (IDs)**: none; by rule no ID cites it.
