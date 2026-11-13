package lifecycle

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrClosed is returned by Group.Run and Group.Add once Run has been called on the Group: a Group runs once,
// as an http.Server is not reused after Shutdown. Match it with errors.Is.
//
// ID:          TB-LIFE-009
// Pattern:     sentinel error
// Authority:   pkg.go.dev net/http, "func (*Server) Shutdown" ("Once Shutdown has been called on a server, it may not be reused")
// Tags:        lifecycle, error, closed
var ErrClosed = errors.New("lifecycle: group already run")

// ErrStopTimeout is matched (errors.Is) by the error Run returns when the grace period ends before every Stop
// has returned. The message names the step still pending ("stop <name> still stopping", or "run <name> still
// running" for a Component.Run not yet returned) and, after "not stopped:", every step never reached.
// It is also the cause (context.Cause) of the ctx given to the Stop that was still running.
//
// ID:          TB-LIFE-010
// Pattern:     sentinel error (drain deadline exceeded)
// Authority:   pkg.go.dev net/http, "func (*Server) Shutdown" ("If the provided context expires before the shutdown is complete, Shutdown returns the context's error")
// Tags:        lifecycle, error, shutdown, deadline
var ErrStopTimeout = errors.New("lifecycle: grace period ended before every component stopped")

// errGoexit is what a Stop (or Run's start) that called runtime.Goexit returns.
var errGoexit = errors.New("lifecycle: runtime.Goexit called")

// timeoutError names the step still pending when the grace period ended and the steps never called. A stop step
// (or Run's start) is "still stopping"; a step waiting for a Component.Run ("run <name>") is "still running".
func timeoutError(pending string, grace time.Duration, notCalled []string) error {
	verb := "stopping"
	if strings.HasPrefix(pending, "run ") {
		verb = "running"
	}
	if len(notCalled) == 0 {
		return fmt.Errorf("%w: %s still %s after %v", ErrStopTimeout, pending, verb, grace)
	}
	return fmt.Errorf("%w: %s still %s after %v; not stopped: %s",
		ErrStopTimeout, pending, verb, grace, strings.Join(notCalled, ", "))
}

// ErrNotReady is matched (errors.Is) by every error Group.Ready returns: before every Start has returned nil
// (also before Run is called and after a Start failed), from the instant shutdown begins, and when a Ready check
// fails (the check errors are wrapped under it). A caller exposing readiness over HTTP maps it to 503.
//
// ID:          TB-LIFE-014
// Pattern:     sentinel error (readiness)
// Authority:   Microsoft Azure Architecture Center, "Health Endpoint Monitoring pattern", "Example > Endpoint behavior" ("Return 200 (OK) when the application can serve requests, or 503 (Service Unavailable) when a required check fails")
// Tags:        lifecycle, error, health, readiness
var ErrNotReady = errors.New("lifecycle: not ready")

// runEnded is the cause of the run ctx when a Run, not the parent ctx, ended the group.
func runEnded(name string, err error) error {
	if err != nil {
		return fmt.Errorf("lifecycle: %s ended the group: %w", name, err)
	}
	return fmt.Errorf("lifecycle: %s ended the group", name)
}
