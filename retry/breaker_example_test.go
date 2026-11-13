package retry_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/CrimsonCoder42/toolbench/breaker"
	"github.com/CrimsonCoder42/toolbench/retry"
)

// retry around breaker: the default WithRetryIf retries breaker.ErrOpen (it is neither a context error nor a panic),
// so a Do around a breaker.Do would wait out every attempt against an open circuit. Reject ErrOpen in the predicate:
// the breaker's answer is final for as long as it stays open.
func ExampleWithRetryIf_breakerOpen() {
	b := breaker.New(breaker.WithFailureThreshold(1)) // one failure opens it
	dials, attempts := 0, 0
	err := retry.Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return b.Do(ctx, func(context.Context) error { dials++; return errFlaky })
	},
		retry.WithClock(newInstant()), retry.WithAttempts(5),
		retry.WithRetryIf(func(err error) bool { return !errors.Is(err, breaker.ErrOpen) }))
	fmt.Println(dials, attempts, errors.Is(err, breaker.ErrOpen), err)
	// Output: 1 2 true breaker: circuit open
}
