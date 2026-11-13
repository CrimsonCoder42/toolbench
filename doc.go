// Package toolbench is a module of standard engineering patterns for Go, each implemented once and shared by
// every project: wherever a project needs fan-out, a worker pool, a retry or a rate limit, it calls toolbench
// instead of hand-rolling it. Consumers import toolbench; toolbench never imports a consumer. All mathematics
// (backoff with jitter, token-bucket refill, bloom-filter sizing, percentiles, running statistics, consistent
// hashing) is called from calcbench (github.com/CrimsonCoder42/calcbench) by its ID, never rewritten here.
//
// This root package holds no code; the patterns live in their own packages, in four layers:
//
//	Layer     Folder                    What lives there                                     Dependencies
//	Core      <pattern>/ at the root    concurrency, resilience, structure, config, logging  standard library + calcbench
//	HTTP      httpx/                    net/http middleware and helpers over core patterns   standard library + core
//	gRPC      grpcx/ (own go.mod)       unary and stream interceptors; status mapping        core + google.golang.org/grpc
//	Adapters  adapters/<driver>/ (own)  thin glue from a core interface to one driver        core + that driver
//
// Core packages import the standard library, calcbench and lower core packages, never httpx, grpcx or an
// adapter. There are no import cycles and no shared grab-bag package. The package map is doc/taxonomy.md;
// every export is listed by ID (TB-<SHORT>-<NNN>) in INDEX.md, which is generated from the doc comments.
package toolbench
