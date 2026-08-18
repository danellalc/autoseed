package autoseed_test

import (
	"testing"

	"github.com/danellalc/autoseed"
)

func TestNewOptions_Defaults(t *testing.T) {
	options := autoseed.NewOptions()
	if options.Seed != 0 {
		t.Fatalf("Seed = %d, want 0", options.Seed)
	}
	if options.Scale != 100 {
		t.Fatalf("Scale = %d, want 100", options.Scale)
	}
	if options.NilRate != 0 {
		t.Fatalf("NilRate = %v, want 0", options.NilRate)
	}
}

func TestNewOptions_AppliesGivenOptions(t *testing.T) {
	options := autoseed.NewOptions(autoseed.WithSeed(42), autoseed.WithScale(1_000), autoseed.WithNilRate(0.25))
	if options.Seed != 42 {
		t.Fatalf("Seed = %d, want 42", options.Seed)
	}
	if options.Scale != 1_000 {
		t.Fatalf("Scale = %d, want 1000", options.Scale)
	}
	if options.NilRate != 0.25 {
		t.Fatalf("NilRate = %v, want 0.25", options.NilRate)
	}
}

func TestWithNilRate_ClampsToUnitInterval(t *testing.T) {
	tests := []struct {
		rate float64
		want float64
	}{
		{rate: -1, want: 0},
		{rate: 0, want: 0},
		{rate: 0.5, want: 0.5},
		{rate: 1, want: 1},
		{rate: 2, want: 1},
	}
	for _, tt := range tests {
		options := autoseed.NewOptions(autoseed.WithNilRate(tt.rate))
		if options.NilRate != tt.want {
			t.Fatalf("WithNilRate(%v): NilRate = %v, want %v", tt.rate, options.NilRate, tt.want)
		}
	}
}
