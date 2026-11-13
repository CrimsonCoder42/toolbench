# Gerrand, "Defer, Panic, and Recover", The Go Blog, 2010

**Citation**: Gerrand, "Defer, Panic, and Recover", The Go Blog, 2010.

**Opened**: https://go.dev/blog/defer-panic-and-recover (opened 2026-11-13 by the panics builder).

**Used for**: Section "Recover": recover regains control of a panicking goroutine and is useful only inside deferred functions; the shape of `panics.Try`.

**Also**: the recover path behind flow.WithPanicHandler.

**Used by (IDs)**: TB-PANIC-001, TB-PANIC-002, TB-PANIC-003, TB-PANIC-004, TB-FLOW-014.
