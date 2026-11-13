# DEFERRED (align_2026-11-13): decided by Devin, NOT built in this run, and where each lands

| item | source | lands in | when |
|---|---|---|---|
| atomic file replace (`WriteAtomic`: temp in the same directory, Sync, Rename, directory Sync, under `os.Root`) | LADDER2 §2.2, §9.1 | `blob` local-filesystem Store, its `Put` | wave 9 |
| saboteur fault-injection helper (deterministic fail/panic decorator for retry, breaker, bulkhead, dead-letter and recover tests) | LADDER2 §2.3, §9.5 | `testkit` | the hardening run (errs, observe, chain, codec, testkit) |
| jobs semantics from three brokers: ack deadline (redeliver after), max-deliver count before dead letter, max-in-flight bound separate from worker count, invalid-message channel distinct from dead-letter channel, dead letter OUTSIDE retry in the middleware order, job status visible to a poller (asynchronous request-reply), requeue carries an attempt counter | LADDER2 §4 jobs; A2 GO-Kafka-MongoDB workers.go:20-92; A4 NATS-go-example main.go:35-40, RabbitMQ worker.go:40-48, torrent p2p.go:160-163 | `doc/specs/jobs.md` (spec first) | wave 8 |
| `httpx.RequestID` middleware (reuse inbound header, generate otherwise, echo on the response, stash with the ctxkey request-id key from D6) | LADDER2 §4 httpx, §9.7 | `httpx` | wave 9 |
| gRPC keepalive ServerParameters as documented defaults; status table (AlreadyExists -> 409, Canceled -> client-closed class, no string matching); interceptor order (tags -> tracing -> metrics -> recover -> log); GracefulStop before Stop | LADDER2 §4 grpcx | `grpcx` | wave 9 |
| `adapters/nats` (ack deadline, max deliver, max ack pending as the jobs Queue over JetStream) | LADDER2 §9.3 | `adapters/nats`, own go.mod | Devin's yes when a project needs NATS |
| `adapters/dynamodb` (store Repository over aws-sdk-go-v2) | LADDER2 §9.4 | `adapters/dynamodb`, own go.mod | Devin's yes when a project needs DynamoDB |
| calcbench requests (Merkle root and proof, Base58, sorted-postings intersection) | LADDER2 §7 | `calcbench_requests.md` in this folder | sent to calcbench by Devin |

Also not in this run (the request's NOT IN THIS RUN): errs, observe, chain, codec, testkit; waves 7+; every
specialist (llm, crawl, auth, blockchain, search, proxy, operator, cli, sse, secretscan); httpx, grpcx, adapters;
any core dependency; `.claude` edits.
