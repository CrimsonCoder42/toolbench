package lifecycle

import "context"

// Component is one part of a process that Group starts and stops: a database pool, a cache, a consumer, a
// server. Start is called once, in Add order, and returns when the component is up (nil) or could not be
// started (an error); it does not block for the component's lifetime. Work that outlives Start runs either in
// Run (below) or on goroutines Start spawns and Stop ends and waits for: derive those from
// context.WithoutCancel(ctx) so that Stop, in reverse order, ends them rather than the signal. Stop is called
// once, in reverse order, only if Start returned nil. A nil Start is a component that needs no start; a nil Stop
// one that needs no stop. Name is used in errors; empty is "component <n>" (n counts from 1 in Add order). The
// zero value is a component that does nothing.
//
// Run is the component's blocking run-time work (a server's Serve, a consumer loop). Group.Run calls it on its
// own goroutine once every Start has returned nil, with a context derived from the run ctx. The first Run to
// return, nil or error, begins the shutdown exactly as ctx being done does (oklog/run: "When the first actor
// returns, all others are interrupted"; errgroup: the first error cancels the Context); its context is cancelled
// when shutdown begins, before the first Stop, with a cause naming who ended the group. A Run that must drain in
// order ignores its ctx and lets its Stop end it, as http.Server's Serve returns when Shutdown is called. After
// the stops, Group.Run waits for every Run within the same grace period. A nil Run is a component with no
// run-time work.
//
// Live and Ready are health checks: Group.Live and Group.Ready call them on the caller's goroutine with the
// caller's ctx and return nil (healthy) or an error. Live answers "is this process alive" regardless of the
// start/stop state; Ready answers "can this component serve" and is only consulted while the group is running.
// A nil check is always healthy.
//
// Run, Live and Ready were added on 2026-11-13: keyed literals (the documented form) are unchanged; an unkeyed
// Component{name, start, stop} literal no longer compiles.
//
// ID:          TB-LIFE-003
// Pattern:     graceful shutdown (ordered start, reverse-order stop, run-time failure seam, health checks)
// Authority:   pkg.go.dev net/http, "func (*Server) Shutdown", "func (*Server) RegisterOnShutdown"
// Authority:   pkg.go.dev golang.org/x/sync/errgroup, "func WithContext" ("canceled the first time a function passed to Go returns a non-nil error")
// Authority:   Microsoft Azure Architecture Center, "Health Endpoint Monitoring pattern", "Solution" ("Deep checks verify the application and the dependencies required to serve requests. Use them for readiness signals")
// Tags:        lifecycle, component, start, stop, run, health
type Component struct {
	Name  string
	Start func(ctx context.Context) error
	Stop  Stopper
	Run   func(ctx context.Context) error
	Live  func(ctx context.Context) error
	Ready func(ctx context.Context) error
}
