package services

import (
	"math"
	"testing"
)

func TestRoundToTwoDecimalPlaces(t *testing.T) {
	tests := []struct{ in, want float64 }{
		{1234.56, 1234.56},
		{1234.5599999, 1234.56}, // truncation used to give 1234.55
		{0.005, 0.01},
		{10, 10},
	}
	for _, tt := range tests {
		if got := roundToTwoDecimalPlaces(tt.in); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("round(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPerUnit(t *testing.T) {
	if got := perUnit(30, 3); got != 10 {
		t.Errorf("perUnit(30,3) = %v", got)
	}
	// A zero quantity must not produce Inf/NaN, which json.Marshal rejects.
	if got := perUnit(30, 0); got != 0 || math.IsNaN(got) || math.IsInf(got, 0) {
		t.Errorf("perUnit(30,0) = %v", got)
	}
}
