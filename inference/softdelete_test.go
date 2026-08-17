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

func TestSoftDeleteRule_Deterministic(t *testing.T) {
	field := autoseed.Field{Name: "DeletedAt", SoftDelete: true, Type: reflect.TypeOf(time.Time{})}

	first := inferField(t, inference.SoftDeleteRule{}, field, 42)
	for i := 0; i < 20; i++ {
		if got := inferField(t, inference.SoftDeleteRule{}, field, 42); got != first {
			t.Fatalf("run %d: SoftDeleteRule = %v, want %v", i, got, first)
		}
	}
}
