# golang/go#23199, "sync: Pool example suggests incorrect usage", 2017

**Citation**: golang/go#23199, "sync: Pool example suggests incorrect usage", 2017.

**Opened**: https://github.com/golang/go/issues/23199 (opened 2026-11-13 by the recycle builder).

**Used for**: A pool that keeps arbitrarily large buffers pins memory; the fix is to drop oversized items instead of pooling them: recycle.WithKeep.

**Used by (IDs)**: TB-RECYC-001, TB-RECYC-005.
