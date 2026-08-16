package inference_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

var timeField = autoseed.Field{Name: "CreatedAt", Type: reflect.TypeOf(time.Time{})}

func TestCreatedAtRule_WithinAFixedSanityWindow(t *testing.T) {
	lower := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	upper := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	seen := make(map[time.Time]bool)
	for seed := uint64(0); seed < 20; seed++ {
		value := inferField(t, inference.CreatedAtRule{}, timeField, seed).(time.Time)
		if value.Before(lower) || value.After(upper) {
			t.Fatalf("seed %d: CreatedAt = %v, want it between %v and %v", seed, value, lower, upper)
		}
		seen[value] = true
	}
	if len(seen) < 2 {
		t.Fatal("20 different seeds produced fewer than 2 distinct CreatedAt values")
	}
}

func TestUpdatedAtRule_AtOrAfterSiblingCreatedAt(t *testing.T) {
	createdAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	generated := map[string]any{"CreatedAt": createdAt}

	for seed := uint64(0); seed < 20; seed++ {
		source := autoseed.NewSeededSource(seed).Entity("E").Row(0).Field("UpdatedAt")
		value := (inference.UpdatedAtRule{}).Infer(autoseed.Field{Name: "UpdatedAt"}, source, generated).(time.Time)
		if value.Before(createdAt) {
			t.Fatalf("seed %d: UpdatedAt = %v is before CreatedAt = %v", seed, value, createdAt)
		}
	}
}

func TestUpdatedAtRule_IndependentWithoutSibling(t *testing.T) {
	source := autoseed.NewSeededSource(1).Entity("E").Row(0).Field("UpdatedAt")
	value := (inference.UpdatedAtRule{}).Infer(autoseed.Field{Name: "UpdatedAt"}, source, nil).(time.Time)
	if value.IsZero() {
		t.Fatal("UpdatedAt must not be the zero time")
	}
}

func TestTemporalRules_Deterministic(t *testing.T) {
	first := inferField(t, inference.CreatedAtRule{}, timeField, 42).(time.Time)
	for i := 0; i < 20; i++ {
		if got := inferField(t, inference.CreatedAtRule{}, timeField, 42).(time.Time); !got.Equal(first) {
			t.Fatalf("run %d: CreatedAt = %v, want %v", i, got, first)
		}
	}
}
