# golang.org/x/sync/singleflight v0.25.0, pkg.go.dev and source

**Citation**: golang.org/x/sync/singleflight v0.25.0, pkg.go.dev and source.

**Opened**: https://pkg.go.dev/golang.org/x/sync/singleflight; https://raw.githubusercontent.com/golang/sync/v0.25.0/singleflight/singleflight.go (opened 2026-11-13 by the singleflight builder); lines 20, 101-104, 148-149, 166-168, 210-214 read in the pinned module-cache copy.

**Used for**: The Group/Do/DoChan/Forget shape; the re-panic and Goexit handling that D-029 replaces with ErrPanic and ErrGoexit. compare/singleflight compares against it.

**Used by (IDs)**: TB-FLIGHT-001, TB-FLIGHT-002, TB-FLIGHT-003.
