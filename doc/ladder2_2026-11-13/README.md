# The second pattern ladder: AkhilSharma90 (2026-11-13)

A second reading list for toolbench, read under the same rules as code-heim (`doc/sources/code-heim.md`,
`.claude/rules/sources.md`): the repositories of github.com/AkhilSharma90 tell us WHICH patterns matter; they are
never copied and never cited. Every pattern is re-researched against today's Go (go1.27,
`doc/ladder_2026-11-13/GO_RELEASES.md`) and its authority before anything is built. Read-only research: this folder
and `_reference/AkhilSharma90/` (gitignored clones) are the only things it wrote; no package, index, taxonomy,
ledger or `.claude/` file was touched, and no Go command was run.

| File | What it is |
|---|---|
| `repos_all.md` | All 192 repositories of the account (GitHub API + profile listing), one row each |
| `repos_go.md` | The 98 kept for cloning (79 language Go, 19 conditional), with clone size and status |
| `repos_dropped.md` | The 94 dropped before cloning (fork or not Go) and the 6 dropped at clone time (no `*.go`) |
| `A1.md` .. `A4.md` | The four readers' rows, one per pattern found, alphabetical quarters of the 92 read |
| `LADDER2.md` | The merge: counts, NEW-GENERAL, improvements to what exists, additions to the plan, specialists, framework and skip, missing from calcbench, agreement with code-heim, decisions |
| `sources_note.md` | Proposed content of `doc/sources/AkhilSharma90.md` (that file belongs to the build session) |

## Discovery and clone (2026-11-13)

- API: `https://api.github.com/users/AkhilSharma90/repos?per_page=100&page=1..3`; pages 1 and 2 returned 100 and
  92 objects, page 3 `[]`. The API did not refuse. The fetch tool truncates each page at about 39 KB, so only 13
  rows carry an API `size`; the rest were completed from the profile listing (7 pages), and sizes were measured
  at clone time with `du -sk`.
- Rule: keep if language Go (or name/description says Go and the tree has `*.go` after cloning), not a fork, not
  archived-and-empty, under 60,000 KB. Nothing is archived; forks: 9; not Go: 85.
- Clone: `git clone --depth 1`, one at a time with `sleep 3`, into `_reference/AkhilSharma90/<name>`.
  98 attempted, 98 succeeded, 0 failed, 0 oversize (largest: golang-restaurant-management-backend 46,092 KB,
  ULTIMATE-GO-Bootcamp-Course 29,964 KB, go-beginner-fiber-HRMS-project 29,096 KB; the large ones hold compiled
  binaries or images, not more code).
- Dropped at clone time, no `*.go`: AI-Neural-Network, Awesome-GO-Hacking, Awesome-GO-LLMs, Distributed-Scraper,
  GO-Projects-To-Contribute-To, go-colly-imdb-crawler.
- Read: 92 repos, split alphabetically (case-folded) into four parts of 23: A1 A-Blockchain-With-Golang ..
  go-chi-CRUD; A2 GO-CQRS-Microservices .. GO-proof-of-work-blockchain; A3 go-ratelimit .. google-scraper;
  A4 google-text-to-speech .. Videocall-Chat-GO-Project.
- `.env` files exist in 7 clones and `vendor/` in one (go-grpc-graphql-microservices); none was opened.

## Result (2026-11-13)

155 rows: COVERED 42 · PLANNED 44 · NEW-GENERAL 2 (keyed mutex, atomic file replace; plus one borderline test
helper) · SPECIALIST 16 · FRAMEWORK 11 · SKIP 40. Merge in `LADDER2.md`; decisions for Devin in its section 9.
