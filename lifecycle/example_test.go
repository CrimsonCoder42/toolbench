package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CrimsonCoder42/toolbench/clock"
	"github.com/CrimsonCoder42/toolbench/lifecycle"
)

func ExampleRun() {
	ctx, cancel := context.WithCancel(context.Background())
	start := func(ctx context.Context) error { // a server's ListenAndServe
		fmt.Println("serving")
		cancel() // in a real main: a signal
		<-ctx.Done()
		return nil
	}
	stopServer := func(context.Context) error { fmt.Println("server drained"); return nil }
	closeDB := func(context.Context) error { fmt.Println("db closed"); return nil }
	err := lifecycle.Run(ctx, 10*time.Second, start, closeDB, stopServer) // stops run last to first
	fmt.Println(err)
	// Output:
	// serving
	// server drained
	// db closed
	// <nil>
}

func ExampleStopper() {
	var stop lifecycle.Stopper = func(ctx context.Context) error { // http.Server.Shutdown has this shape
		fmt.Println("draining")
		return nil
	}
	err := lifecycle.Run(context.Background(), time.Second, func(context.Context) error { return nil }, stop)
	fmt.Println(err)
	// Output:
	// draining
	// <nil>
}

// component prints its start and stop.
func component(name string) lifecycle.Component {
	return lifecycle.Component{
		Name:  name,
		Start: func(context.Context) error { fmt.Println("start", name); return nil },
		Stop:  func(context.Context) error { fmt.Println("stop", name); return nil },
	}
}

func ExampleGroup() {
	ctx, cancel := context.WithCancel(context.Background())
	var g lifecycle.Group
	_ = g.Add(component("db"))
	_ = g.Add(component("cache"))
	_ = g.Add(component("server"))
	_ = g.Add(lifecycle.Component{Start: func(context.Context) error { cancel(); return nil }}) // a signal
	fmt.Println(g.Run(ctx, lifecycle.WithGrace(10*time.Second)))
	// Output:
	// start db
	// start cache
	// start server
	// stop server
	// stop cache
	// stop db
	// <nil>
}

func ExampleGroup_Add() {
	var g lifecycle.Group
	_ = g.Add(component("db"))
	_ = g.Add(lifecycle.Component{Name: "server", Start: func(context.Context) error { return errors.New("port in use") }})
	fmt.Println(g.Run(context.Background()))
	fmt.Println(g.Add(component("late")))
	// Output:
	// start db
	// stop db
	// lifecycle: start server: port in use
	// lifecycle: group already run
}

func ExampleGroup_Run() {
	var g lifecycle.Group
	_ = g.Add(component("db"))
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("shutting down")) // done before the first Start
	fmt.Println(g.Run(ctx))
	// Output:
	// shutting down
}

func ExampleComponent() {
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{}) // the zero value does nothing
	_ = g.Add(lifecycle.Component{Start: func(context.Context) error { return errors.New("no config") }})
	fmt.Println(g.Run(context.Background()))
	// Output:
	// lifecycle: start component 2: no config
}

func ExampleOption() {
	ctx, cancel := context.WithCancel(context.Background())
	opts := []lifecycle.Option{lifecycle.WithGrace(5 * time.Second), lifecycle.WithClock(clock.Real())}
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Start: func(context.Context) error { cancel(); return nil }})
	fmt.Println(g.Run(ctx, opts...))
	// Output:
	// <nil>
}

func ExampleWithGrace() {
	ctx, cancel := context.WithCancel(context.Background())
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{
		Name:  "server",
		Start: func(context.Context) error { cancel(); return nil },
		Stop: func(ctx context.Context) error {
			<-ctx.Done() // a drain that does not finish in time
			return ctx.Err()
		},
	})
	err := g.Run(ctx, lifecycle.WithGrace(time.Millisecond))
	fmt.Println(errors.Is(err, lifecycle.ErrStopTimeout))
	// Output:
	// true
}

func ExampleWithClock() {
	fc := clock.NewFake(time.Unix(0, 0))
	ctx, cancel := context.WithCancel(context.Background())
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{
		Name:  "slow",
		Start: func(context.Context) error { cancel(); return nil }, // started, then a signal
		Stop:  func(ctx context.Context) error { <-ctx.Done(); return nil },
	})
	done := make(chan error, 1)
	go func() { done <- g.Run(ctx, lifecycle.WithClock(fc), lifecycle.WithGrace(time.Minute)) }()
	_ = fc.BlockUntil(context.Background(), 1) // shutdown has armed the grace timer
	fc.Advance(time.Minute)
	fmt.Println(<-done)
	// Output:
	// lifecycle: grace period ended before every component stopped: stop slow still stopping after 1m0s
}

func ExampleWithNotify() {
	// In a main: return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM).
	notify := func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		cancel() // the signal has already arrived
		return ctx, cancel
	}
	var g lifecycle.Group
	_ = g.Add(component("db"))
	fmt.Println(g.Run(context.Background(), lifecycle.WithNotify(notify)))
	// Output:
	// context canceled
}

func ExampleErrClosed() {
	var g lifecycle.Group
	_ = g.Run(context.Background(), lifecycle.WithNotify(func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return ctx, cancel
	}))
	fmt.Println(errors.Is(g.Run(context.Background()), lifecycle.ErrClosed))
	// Output:
	// true
}

func ExampleErrStopTimeout() {
	release := make(chan struct{})
	defer close(release)
	err := lifecycle.Run(context.Background(), time.Millisecond, func(context.Context) error { return nil },
		func(context.Context) error { <-release; return nil }) // ignores its ctx
	fmt.Println(errors.Is(err, lifecycle.ErrStopTimeout))
	fmt.Println(err)
	// Output:
	// true
	// lifecycle: grace period ended before every component stopped: stop 1 still stopping after 1ms
}

func ExampleDefaultGrace() {
	fmt.Println(lifecycle.DefaultGrace)
	// Output:
	// 30s
}

func ExampleGroup_run() {
	var g lifecycle.Group
	_ = g.Add(component("db"))
	up := make(chan struct{})     // closed once the server is serving
	closed := make(chan struct{}) // closed by the server's Stop, as Shutdown makes Serve return
	_ = g.Add(lifecycle.Component{
		Name: "server",
		Run: func(context.Context) error { // blocks like srv.Serve; ignores ctx and lets Stop end it
			fmt.Println("serving")
			close(up)
			<-closed
			return nil
		},
		Stop: func(context.Context) error { fmt.Println("stop server"); close(closed); return nil },
	})
	_ = g.Add(lifecycle.Component{
		Name: "consumer",
		Run:  func(context.Context) error { <-up; return errors.New("broker gone") }, // the first Run to return ends the group
	})
	fmt.Println(g.Run(context.Background()))
	// Output:
	// start db
	// serving
	// stop server
	// stop db
	// lifecycle: run consumer: broker gone
}

func ExampleGroup_ready() {
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "db", Ready: func(context.Context) error { return nil }}) // a ping
	_ = g.Add(lifecycle.Component{
		Name:  "server",
		Start: func(ctx context.Context) error { fmt.Println("starting:", g.Ready(ctx)); return nil },
		Run:   func(ctx context.Context) error { fmt.Println("running:", g.Ready(ctx)); return nil }, // returning ends the group
		Stop:  func(ctx context.Context) error { fmt.Println("stopping:", g.Ready(ctx)); return nil },
	})
	fmt.Println("before:", g.Ready(context.Background()))
	fmt.Println(g.Run(context.Background()))
	// Output:
	// before: lifecycle: not ready: not every component has started
	// starting: lifecycle: not ready: not every component has started
	// running: <nil>
	// stopping: lifecycle: not ready: shutting down
	// <nil>
}

func ExampleGroup_Live() {
	var g lifecycle.Group
	_ = g.Add(lifecycle.Component{Name: "db", Live: func(context.Context) error { return nil }})
	_ = g.Add(lifecycle.Component{Name: "cache", Live: func(context.Context) error { return errors.New("no pong") }})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second) // the probe's deadline: the caller sets it
	defer cancel()
	fmt.Println(g.Live(ctx))
	// Output:
	// lifecycle: live cache: no pong
}

func ExampleErrNotReady() {
	var g lifecycle.Group
	err := g.Ready(context.Background()) // Run has not been called
	fmt.Println(errors.Is(err, lifecycle.ErrNotReady), err)
	// Output:
	// true lifecycle: not ready: not every component has started
}
