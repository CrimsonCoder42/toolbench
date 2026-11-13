// Package calccheck_test proves the link to calcbench from day one: it is the only place calcbench is imported
// until the first pattern lands (doc/requests/foundation_2026-11-13.md, A4).
package calccheck_test

import (
	"testing"

	"github.com/CrimsonCoder42/calcbench/num/stats"
)

// TestArithmeticMean_linksCalcbench calls NUM-STATS-001 on 1, 2, 3 and expects 2.
func TestArithmeticMean_linksCalcbench(t *testing.T) {
	got, err := stats.ArithmeticMean([]float64{1, 2, 3})
	if err != nil {
		t.Fatalf("NUM-STATS-001 ArithmeticMean(1, 2, 3): %v", err)
	}
	if got != 2 {
		t.Fatalf("NUM-STATS-001 ArithmeticMean(1, 2, 3) = %v, want 2", got)
	}
}
