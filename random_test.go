package autoseed_test

import (
	"testing"

	"github.com/danellalc/autoseed"
)

func draw(seed uint64, entity string, row int, field string) uint64 {
	return autoseed.NewSeededSource(seed).Entity(entity).Row(row).Field(field).Rand().Uint64()
}

func TestSeededSource_Deterministic(t *testing.T) {
	first := draw(42, "Order", 5, "Total")
	for i := 0; i < 100; i++ {
		if got := draw(42, "Order", 5, "Total"); got != first {
			t.Fatalf("run %d: got %d, want %d", i, got, first)
		}
	}
}

func TestSeededSource_IsolatedRowMatchesBatchRow(t *testing.T) {
	source := autoseed.NewSeededSource(7).Entity("Order")

	var inBatch uint64
	for row := 0; row <= 500; row++ {
		v := source.Row(row).Field("Total").Rand().Uint64()
		if row == 500 {
			inBatch = v
		}
	}

	isolated := autoseed.NewSeededSource(7).Entity("Order").Row(500).Field("Total").Rand().Uint64()
	if isolated != inBatch {
		t.Fatalf("isolated row 500 = %d, want %d (same as generated inside the batch)", isolated, inBatch)
	}
}

func TestSeededSource_DifferentPositionsDiverge(t *testing.T) {
	positions := []struct {
		entity string
		row    int
		field  string
	}{
		{"Order", 0, "Total"},
		{"Order", 1, "Total"},
		{"Order", 0, "Status"},
		{"Customer", 0, "Total"},
	}

	seen := make(map[uint64]string, len(positions))
	for _, p := range positions {
		v := draw(42, p.entity, p.row, p.field)
		key := p.entity + string(rune(p.row)) + p.field
		if other, ok := seen[v]; ok {
			t.Fatalf("collision between %q and %q", other, key)
		}
		seen[v] = key
	}
}

func TestSeededSource_DifferentSeedsDiverge(t *testing.T) {
	if draw(1, "Order", 0, "Total") == draw(2, "Order", 0, "Total") {
		t.Fatal("different root seeds produced the same value")
	}
}
