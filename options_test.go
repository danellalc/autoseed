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
}

func TestNewOptions_AppliesGivenOptions(t *testing.T) {
	options := autoseed.NewOptions(autoseed.WithSeed(42), autoseed.WithScale(1_000))
	if options.Seed != 42 {
		t.Fatalf("Seed = %d, want 42", options.Seed)
	}
	if options.Scale != 1_000 {
		t.Fatalf("Scale = %d, want 1000", options.Scale)
	}
}
