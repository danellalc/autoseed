package inference_test

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

func orderItemEntity() autoseed.Entity {
	return autoseed.Entity{
		Name: "OrderItem",
		Fields: []autoseed.Field{
			{Name: "ID", Type: reflect.TypeOf(0), PrimaryKey: true, AutoIncrement: true},
			{Name: "OrderID", Type: reflect.TypeOf(0)},
			{Name: "FirstName", Type: reflect.TypeOf("")},
			{Name: "LastName", Type: reflect.TypeOf("")},
			{Name: "Email", Type: reflect.TypeOf("")},
			{Name: "SKU", Type: reflect.TypeOf(""), Size: 10},
			{Name: "Price", Type: reflect.TypeOf(float64(0))},
			{Name: "Quantity", Type: reflect.TypeOf(0)},
			{Name: "Total", Type: reflect.TypeOf(float64(0))},
			{Name: "CreatedAt", Type: reflect.TypeOf(time.Time{})},
			{Name: "UpdatedAt", Type: reflect.TypeOf(time.Time{})},
		},
		References: []autoseed.Reference{
			{Fields: []string{"OrderID"}, Target: "Order"},
		},
	}
}

// TestDefaultGenerator_ByteIdenticalAcrossRuns is the determinism guarantee
// PLANO.md's Fase 2 exists to prove: the same seed produces the same data,
// field for field, run after run.
func TestDefaultGenerator_ByteIdenticalAcrossRuns(t *testing.T) {
	entity := orderItemEntity()

	first, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(42).Entity(entity.Name).Row(0))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}

	for i := 0; i < 50; i++ {
		got, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(42).Entity(entity.Name).Row(0))
		if err != nil {
			t.Fatalf("run %d: GenerateRow: %v", i, err)
		}
		for _, field := range entity.Fields {
			if field.PrimaryKey {
				continue
			}
			want := fmt.Sprintf("%v", first[field.Name])
			gotValue := fmt.Sprintf("%v", got[field.Name])
			if gotValue != want {
				t.Fatalf("run %d: %s = %v, want %v", i, field.Name, got[field.Name], first[field.Name])
			}
		}
	}
}

func TestDefaultGenerator_CoherenceHoldsTogether(t *testing.T) {
	entity := orderItemEntity()

	for seed := uint64(0); seed < 20; seed++ {
		values, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(seed).Entity(entity.Name).Row(0))
		if err != nil {
			t.Fatalf("seed %d: GenerateRow: %v", seed, err)
		}

		price := values["Price"].(float64)
		quantity := values["Quantity"].(int)
		total := values["Total"].(float64)
		wantTotal := math.Round(price*float64(quantity)*100) / 100
		if total != wantTotal {
			t.Fatalf("seed %d: Total = %v, want Price*Quantity = %v", seed, total, wantTotal)
		}

		createdAt := values["CreatedAt"].(time.Time)
		updatedAt := values["UpdatedAt"].(time.Time)
		if updatedAt.Before(createdAt) {
			t.Fatalf("seed %d: UpdatedAt = %v is before CreatedAt = %v", seed, updatedAt, createdAt)
		}
	}
}
