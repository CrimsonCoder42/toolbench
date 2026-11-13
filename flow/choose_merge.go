package flow

import (
	"context"
	"sync"
)

// arrival is one receive done by a receiver goroutine on input i: a value (ok) or the close of the input.
type arrival[T any] struct {
	i  int
	v  T
	ok bool
}

// chooseMerge is the merger behind Priority and Weighted. Each turn it polls every open input that holds no
// value, without blocking, so each input holds at most one value; then choose picks one input among those
// holding a value (ready, in ascending index order, never empty) and that value is sent on out. When no input
// holds a value, it starts one receiver goroutine per open input and waits for the first arrival or ctx; then it
// stops every receiver, waits for all of them, and records every value they took before choosing, so a value
// received from a higher input is never passed over for a lower one. A receiver receives at most one value (or
// the close) into arrivals, whose capacity is len(ins), so it never blocks after that.
//
// It returns when every input is closed and every held value is sent, or when ctx is done (held values, and a
// value a receiver had just taken, are dropped). No receiver is left running when it returns; the caller closes
// out after that.
func chooseMerge[T any](ctx context.Context, ins []<-chan T, out chan<- T, choose func(ready []int) int) {
	n := len(ins)
	var (
		held     = make([]T, n)
		holding  = make([]bool, n)
		open     = make([]bool, n)
		ready    = make([]int, 0, n)
		live     = 0 // open inputs; only an open input holds a value
		arrivals = make(chan arrival[T], n)
	)
	for i, in := range ins {
		if in != nil {
			open[i] = true
			live++
		}
	}
	record := func(a arrival[T]) {
		if a.ok {
			held[a.i], holding[a.i] = a.v, true
			return
		}
		open[a.i] = false
		live--
	}
	for live > 0 {
		if ctx.Err() != nil {
			return
		}
		for i, in := range ins {
			if !open[i] || holding[i] {
				continue
			}
			select {
			case v, ok := <-in:
				record(arrival[T]{i: i, v: v, ok: ok})
			default:
			}
		}
		ready = ready[:0]
		for i := range n {
			if holding[i] {
				ready = append(ready, i)
			}
		}
		if len(ready) > 0 {
			i := choose(ready)
			v := held[i]
			var zero T
			held[i], holding[i] = zero, false
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
			continue
		}
		if live == 0 {
			return
		}
		// Nothing ready: one receiver per open input, until the first value or close on any of them.
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for i, in := range ins {
			if !open[i] {
				continue
			}
			wg.Go(func() {
				select {
				case v, ok := <-in:
					arrivals <- arrival[T]{i: i, v: v, ok: ok}
				case <-stop:
				}
			})
		}
		select {
		case a := <-arrivals:
			record(a)
		case <-ctx.Done():
		}
		close(stop)
		wg.Wait()
		// Every value a receiver took is in arrivals now; hold them all before the next choice.
	drain:
		for {
			select {
			case a := <-arrivals:
				record(a)
			default:
				break drain
			}
		}
	}
}
