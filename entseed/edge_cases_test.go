package entseed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/entseed"
)

// TestSeed_NilClient and TestSeedCoverage_NilClient guard clientValueOf,
// the helper both Seed and SeedCoverage share since persist.go's
// resolveModel/seedPlan refactor -- fast, no Docker involved, since a nil
// client is rejected before resolveModel ever touches the schema.
func TestSeed_NilClient(t *testing.T) {
	err := entseed.Seed(context.Background(), nil, schemaPath, autoseed.WithScale(10))
	if !errors.Is(err, entseed.ErrNilClient) {
		t.Fatalf("Seed(nil client) = %v, want ErrNilClient", err)
	}
}

func TestSeedCoverage_NilClient(t *testing.T) {
	err := entseed.SeedCoverage(context.Background(), nil, schemaPath)
	if !errors.Is(err, entseed.ErrNilClient) {
		t.Fatalf("SeedCoverage(nil client) = %v, want ErrNilClient", err)
	}
}
