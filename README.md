# toolbench

Standard engineering patterns for Go, each implemented once: concurrency (fan-out/fan-in, pipelines, worker
pools), resilience (retry, backoff, rate limiting, circuit breaking), structure, configuration and logging.
The core uses only the standard library and [calcbench](https://github.com/CrimsonCoder42/calcbench), which
supplies the mathematics. Transport and driver code lives in separate modules (`httpx`, `grpcx`,
`adapters/<driver>`), so a project imports only what it uses.

    go get github.com/CrimsonCoder42/toolbench

`INDEX.md` lists every export by ID. `doc/taxonomy.md` is the package map.
