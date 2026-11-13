package singleflight

import "context"

// Result holds the outcome of one call as DoChan delivers it: the value, the error and whether more than one
// caller joined the call. Val is shared memory when Shared is true: do not mutate it. The zero value is an
// empty result (zero Val, nil Err, Shared false).
//
// ID:          TB-FLIGHT-002
// Pattern:     duplicate call suppression (singleflight)
// Authority:   pkg.go.dev golang.org/x/sync/singleflight v0.25.0, "Result"
// Tags:        concurrency, singleflight, result, generic
type Result[V any] struct {
	Val    V
	Err    error
	Shared bool
}

// DoChan is like Do but returns at once with a channel that receives exactly one Result and is then closed
// (x/sync's channel is never closed). The caller joins the call in flight for key, or starts it, before DoChan
// returns. The channel has a buffer of one, so a caller that never reads it blocks nothing; the goroutine that
// delivers the Result exits once it has sent it. If ctx is done before DoChan, the Result carries
// context.Cause(ctx) and no goroutine is started. Cancellation, errors and sharing are as for Do.
func (g *Group[K, V]) DoChan(ctx context.Context, key K, fn func(context.Context) (V, error)) <-chan Result[V] {
	if ctx == nil {
		ctx = context.Background()
	}
	ch := make(chan Result[V], 1)
	if ctx.Err() != nil {
		ch <- Result[V]{Err: context.Cause(ctx)}
		close(ch)
		return ch
	}
	c := g.join(ctx, key, fn)
	go func() {
		defer close(ch)
		v, shared, err := g.wait(ctx, key, c)
		ch <- Result[V]{Val: v, Err: err, Shared: shared}
	}()
	return ch
}
