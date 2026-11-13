// Package lifecycle starts the parts of a process in order and stops them gracefully, in reverse, within a
// drain deadline: the run group of a main function. Group.Run calls each Component's Start in the order it was
// added, waits until its ctx is done (a signal, through WithNotify), then calls the Stop of every started
// component in reverse order under one grace deadline, as net/http Server.Shutdown drains with a deadline. A
// Start that fails stops the components already started, in reverse. A Component.Run (a server's Serve, a consumer
// loop) runs on its own goroutine once every Start has returned; the first Run to return, nil or error, begins the
// shutdown as ctx being done does, and the remaining Runs are waited for within the same grace period. Run is the
// one-component form: a blocking start (a server's Serve) and the stops that end it. Group.Live and Group.Ready
// aggregate the components' health checks under the caller's ctx; Ready is false until every Start has returned
// and from the instant shutdown begins, so a probing load balancer stops routing while the components drain.
//
// Stop errors are all returned (errors.Join), each naming its component. A component that has not stopped when
// the grace period ends is named in an ErrStopTimeout error and Run returns at once: the process is about to
// exit, and a stuck Stop must not hold it (The Twelve-Factor App, "IX. Disposability"). A panic in Start, Stop
// or the notify hook is recovered and returned as panics.ErrPanic. Nothing here panics on input: a nil ctx is
// context.Background(), a non-positive grace is DefaultGrace, a nil Start or Stop does nothing, a second Run is
// ErrClosed. The package never imports os/signal: the caller passes signal.NotifyContext through WithNotify.
//
// ID short form: LIFE (IDs are TB-LIFE-NNN).
//
// Exports:
//
//	TB-LIFE-001  Stopper         a function that stops one component within the deadline of its ctx
//	TB-LIFE-002  Run             runs a blocking start until it returns or ctx is done, then the stops in reverse
//	TB-LIFE-003  Component       one part of a process: a Name, a Start, a Stop, a run-time Run and Live/Ready health checks
//	TB-LIFE-004  Group           a run group: ordered start, reverse-order stop, drain deadline
//	TB-LIFE-005  Option          a functional option for Group.Run
//	TB-LIFE-006  WithGrace       the drain deadline for every Stop together (default DefaultGrace)
//	TB-LIFE-007  WithClock       the clock that measures the grace period (default clock.Real())
//	TB-LIFE-008  WithNotify      the shutdown hook, for example signal.NotifyContext (default none)
//	TB-LIFE-009  ErrClosed       returned by Run and Add once a Group has been run
//	TB-LIFE-010  ErrStopTimeout  the grace period ended before every component stopped
//	TB-LIFE-011  DefaultGrace    the grace period used when none (or a non-positive one) is given: 30s
//	TB-LIFE-012  Group.Live            Group.Live: every component's liveness check, one at a time, errors joined and named
//	TB-LIFE-013  Group.Ready           Group.Ready: ErrNotReady while starting or stopping, else every readiness check
//	TB-LIFE-014  ErrNotReady     matched by every error Group.Ready returns (starting, shutting down, a failing check)
package lifecycle
