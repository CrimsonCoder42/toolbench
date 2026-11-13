# calcbench requests from the align run (2026-11-13)

Checked against `/Users/devinanderson/Desktop/calcbench/INDEX.md` (v0.3.0 pinned, D-034) on 2026-11-13 before
asking. Nothing here is built in toolbench.

| R | what | signature wanted | authority | needed by | checked |
|---|---|---|---|---|---|
| R-6 | Merkle root and inclusion (audit) proof over leaves `[][]byte` with a `hash.Hash` constructor, RFC 6962 leaf prefix 0x00 and node prefix 0x01, odd-leaf rule (promote, do not duplicate) | `algo/tree`: `MerkleRoot(h func() hash.Hash, leaves [][]byte) ([]byte, error)`; `MerkleProof(h, leaves, i) ([][]byte, error)`; `VerifyMerkleProof(h, root, leaf []byte, i, n int, proof [][]byte) (bool, error)` | RFC 6962 §2.1 "Merkle Hash Trees", §2.1.1 "Merkle Audit Paths"; Merkle, "A Digital Signature Based on a Conventional Encryption Function", CRYPTO 1987 | a `blockchain` specialist only (LADDER2 §5); low priority | no `merkle`/`hash tree` row in INDEX.md; ALG-TEXT-004 RollingHash is unrelated |
| R-7 | Base58 encode/decode (Bitcoin alphabet, leading-zero bytes as leading '1's) | `algo/encoding`: `Base58Encode(b []byte) string`; `Base58Decode(s string) ([]byte, error)` | Bitcoin Core `base58.cpp` reference description; Nakamoto 2008 (the use) | a `blockchain` specialist only; low priority | no `base58` row in INDEX.md |
| R-8 | Sorted-postings intersection: the common elements of two ascending `[]int` (or `[]T cmp.Ordered`), two-pointer merge, O(n+m) | `algo/seq` or `algo/sorting`: `IntersectSorted[T cmp.Ordered](a, b []T) ([]T, error)` (error for an unsorted input) | Manning, Raghavan, Schütze, Introduction to Information Retrieval, 2008, §1.3 "Processing Boolean queries" (Figure 1.6, INTERSECT); Knuth TAOCP Vol. 3, §5.2.4 (two-way merge) | a `search` specialist only; low priority | ALG-INTVL-003 IntersectLists is over `[]Interval[T]` (ranges), not points, so it is NOT this; ALG-HASH-006 SetIntersection is hash-based, unordered output |

Already present (LADDER's R-1..R-5 are shipped: ALG-BACKOF-001..004, ALG-CRON-001/002, NUM-INTEG-013,
ALG-RATELM-005, ALG-LIST-018). The request-id helper (D6) found no id or random-string primitive in INDEX.md
(ALG-SAMPLE-* draw from a source the caller supplies; none produces an identifier), so it uses the standard
library (`crypto/rand`, documented in `doc/specs/ctxkey.md`); no request is raised for it, because an id is not
mathematics.
| R-9 | Vixie cron step values `*\/N` and `a-b\/N` (and, lower priority, names `MON`\/`JAN` and `@daily` macros) as an additive extension of the POSIX grammar in ALG-CRON-001 | `algo\/cron`: `Parse` accepts `*\/10 * * * *`; `Next` unchanged | crontab(5) (Vixie cron, "step values"); Kubernetes CronJob schedule syntax; robfig\/cron v3 parser (familiarity) | `schedule.ParseCron` TB-SCHED-012 (go_49 main.go:59 `*\/10 * * * *` is rejected today: documented ErrSyntax); REPEAT 3 | ALG-CRON-001 is POSIX-only by calcbench A-069; no steps |
