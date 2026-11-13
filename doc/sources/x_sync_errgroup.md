# golang.org/x/sync/errgroup v0.25.0, pkg.go.dev and source

**Citation**: golang.org/x/sync/errgroup v0.25.0, pkg.go.dev and source.

**Opened**: https://pkg.go.dev/golang.org/x/sync/errgroup; raw.githubusercontent.com/golang/sync at v0.25.0, v0.16.0, v0.15.0 and master, errgroup/errgroup.go; commits 506c70f and 7fad2c9 (opened 2026-11-13 by the group builder).

**Used for**: The API shape group mirrors (Group, WithContext, Go, TryGo, SetLimit, Wait) and the finding of D-017 (no re-panic from Wait since v0.16.0). compare/group compares against it. Align run 2026-11-13: errgroup first-error default kept beside SetCollect; "first to return ends the group" for Component.Run.

**Used by (IDs)**: TB-GROUP-001, TB-GROUP-002, TB-GROUP-003, TB-GROUP-004, TB-GROUP-005, TB-GROUP-006, TB-GROUP-007, TB-POOL-005, TB-GROUP-008, TB-LIFE-003, TB-LIFE-004.
