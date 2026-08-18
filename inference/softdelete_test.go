package inference_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

func TestSoftDeleteRule_CanInfer(t *testing.T) {
	tests := []struct {
		name  string
		field autoseed.Field
		want  bool
	}{
		{"soft delete flag set", autoseed.Field{Name: "DeletedAt", SoftDelete: true}, true},
		{"soft delete flag unset despite matching name", autoseed.Field{Name: "DeletedAt", SoftDelete: false}, false},
		{"soft delete flag set on an unrelated name", autoseed.Field{Name: "Archived", SoftDelete: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (inference.SoftDeleteRule{}).CanInfer(tt.field); got != tt.want {
				t.Fatalf("CanInfer(%+v) = %v, want %v", tt.field, got, tt.want)
			}
		})
	}
}

func TestSoftDeleteRule_MostlyNilWithAMinorityDeleted(t *testing.T) {
	field := autoseed.Field{Name: "DeletedAt", SoftDelete: true, Type: reflect.TypeOf(time.Time{})}

	nilCount, deletedCount := 0, 0
	for seed := uint64(0); seed < 1000; seed++ {
		value := inferField(t, inference.SoftDeleteRule{}, field, seed)
		if value == nil {
			nilCount++
			continue
		}
		deletedCount++
		if value.(time.Time).After(time.Now()) {
			t.Fatalf("seed %d: deleted-at timestamp %v is in the future", seed, value)
		}
	}

	if deletedCount == 0 {
		t.Fatal("1000 draws never produced a deleted row, want a minority marked deleted")
	}
	if nilCount <= deletedCount {
		t.Fatalf("nil (not deleted) count %d is not the large majority over deleted count %d", nilCount, deletedCount)
	}
}

// TestSoftDeleteRule_NeverBeforeSiblingUpdatedAt guards a real bug: a row
// cannot be deleted before it was last touched. Infer used to discard the
// generated map entirely and draw an independent past timestamp, so a
// deleted row could easily land before its own CreatedAt/UpdatedAt.
func TestSoftDeleteRule_NeverBeforeSiblingUpdatedAt(t *testing.T) {
	field := autoseed.Field{Name: "DeletedAt", SoftDelete: true}
	createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	generated := map[string]any{"CreatedAt": createdAt, "UpdatedAt": updatedAt}

	sawDeleted := false
	for seed := uint64(0); seed < 500; seed++ {
		source := autoseed.NewSeededSource(seed).Entity("E").Row(0).Field("DeletedAt")
		value := (inference.SoftDeleteRule{}).Infer(field, source, generated)
		if value == nil {
			continue
		}
		sawDeleted = true
		if value.(time.Time).Before(updatedAt) {
			t.Fatalf("seed %d: DeletedAt = %v is before sibling UpdatedAt = %v", seed, value, updatedAt)
		}
	}
	if !sawDeleted {
		t.Fatal("500 draws never produced a deleted row")
	}
}

// TestSoftDeleteRule_FallsBackToCreatedAtWithoutUpdatedAt guards the
// fallback path when only CreatedAt was generated on the same row.
func TestSoftDeleteRule_FallsBackToCreatedAtWithoutUpdatedAt(t *testing.T) {
	field := autoseed.Field{Name: "DeletedAt", SoftDelete: true}
	createdAt := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	generated := map[string]any{"CreatedAt": createdAt}

	sawDeleted := false
	for seed := uint64(0); seed < 500; seed++ {
		source := autoseed.NewSeededSource(seed).Entity("E").Row(0).Field("DeletedAt")
		value := (inference.SoftDeleteRule{}).Infer(field, source, generated)
		if value == nil {
			continue
		}
		sawDeleted = true
		if value.(time.Time).Before(createdAt) {
			t.Fatalf("seed %d: DeletedAt = %v is before sibling CreatedAt = %v", seed, value, createdAt)
		}
	}
	if !sawDeleted {
		t.Fatal("500 draws never produced a deleted row")
	}
}

func TestSoftDeleteRule_Deterministic(t *testing.T) {
	field := autoseed.Field{Name: "DeletedAt", SoftDelete: true, Type: reflect.TypeOf(time.Time{})}

	first := inferField(t, inference.SoftDeleteRule{}, field, 42)
	for i := 0; i < 20; i++ {
		if got := inferField(t, inference.SoftDeleteRule{}, field, 42); got != first {
			t.Fatalf("run %d: SoftDeleteRule = %v, want %v", i, got, first)
		}
	}
}
