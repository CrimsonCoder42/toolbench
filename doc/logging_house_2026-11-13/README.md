# Logging house format (2026-11-13)

Request: `doc/requests/logging_house_2026-11-13.md`. Adds Devin's house logging display (origin: backendAdvo
`util/logger.go`) to the existing `logging` package as slog handlers and helpers, plus the project-wide coding
standard `doc/standards/logging.md`. No released signature changes; no logger type of our own (log/slog is the
logger).

| File | What it is |
|---|---|
| `PROGRESS.md` | What is finished (a rerun skips it) |
| `out/` | Raw test output (gitignored) |

Spec: `doc/specs/logging.md` ("House format"). Standard: `doc/standards/logging.md`.
