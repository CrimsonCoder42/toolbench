package parallel

import (
	"context"
	"errors"
	"iter"
	"runtime"
	"sync"

	"github.com/CrimsonCoder42/toolbench/flow"
	"github.com/CrimsonCoder42/toolbench/panics"
)

// indexed carries a value with its position in the input, so results finishing out of order can be put back
// in input order.
type indexed[T any] struct {
	i int
	v T
}

// run is the engine of Map, Filter and ForEach. It numbers the values of in, sends them from one flow.FromSeq
// goroutine to a flow.FanOut of workers goroutines running fn, and calls emit(i, result) on the caller's
// goroutine for every result, in completion order. It returns when every goroutine it started has exited.
//
// The first error or panic from fn is recorded before the shared ctx is cancelled, so an error that another
// call returns because it saw the cancellation never replaces it. After it, in is advanced by at most the one
// value FromSeq is holding plus one value per worker that was waiting to receive; yield then returns false.
func run[In, Out any](ctx context.Context, in iter.Seq[In], workers int, fn func(context.Context, In) (Out, error), emit func(int, Out)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return context.Cause(ctx) // nothing started, in not ranged, fn not called
	}
	if in == nil {
		return nil
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		once  sync.Once
		first error // written once before cancel; read after waitFan, which orders it
	)
	numbered := func(yield func(indexed[In]) bool) {
		i := 0
		for v := range in {
			if !yield(indexed[In]{i, v}) {
				return
			}
			i++
		}
	}
	src, waitSrc := flow.FromSeq(rctx, numbered)
	out, waitFan := flow.FanOut(rctx, src, workers, func(c context.Context, x indexed[In]) (indexed[Out], error) {
		if err := c.Err(); err != nil {
			return indexed[Out]{}, err // fn is never started after cancellation is seen
		}
		r, err := panics.TryValue(func() (Out, error) { return fn(c, x.v) })
		if err != nil {
			once.Do(func() { first = err })
			cancel()
			return indexed[Out]{}, err
		}
		return indexed[Out]{x.i, r}, nil
	})
	for r := range out {
		emit(r.i, r.v)
	}
	cancel() // unblocks FromSeq if it holds a value nobody will receive
	srcErr := waitSrc()
	fanErr := waitFan()

	switch {
	case first != nil:
		if ctx.Err() != nil && errors.Is(first, ctx.Err()) {
			return context.Cause(ctx)
		}
		return first
	case errors.Is(srcErr, panics.ErrPanic):
		return srcErr
	case fanErr != nil || srcErr != nil:
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		return errors.Join(fanErr, srcErr) // unreachable: work is only cut short by ctx or by first
	}
	return nil
}
