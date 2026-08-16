package inference_test

import (
	"reflect"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

func TestPriceRule_WithinReasonableRange(t *testing.T) {
	field := autoseed.Field{Name: "Price", Type: reflect.TypeOf(float64(0))}
	for seed := uint64(0); seed < 20; seed++ {
		value := inferField(t, inference.PriceRule{}, field, seed).(float64)
		if value < 1.00 || value > 999.99 {
			t.Fatalf("seed %d: Price = %v, want it between 1.00 and 999.99", seed, value)
		}
	}
}

func TestQuantityRule_WithinReasonableRange(t *testing.T) {
	field := autoseed.Field{Name: "Quantity", Type: reflect.TypeOf(0)}
	for seed := uint64(0); seed < 20; seed++ {
		value := inferField(t, inference.QuantityRule{}, field, seed).(int)
		if value < 1 || value > 20 {
			t.Fatalf("seed %d: Quantity = %v, want it between 1 and 20", seed, value)
		}
	}
}

func TestCorrelatedTotalRule_EqualsSiblingPriceTimesQuantity(t *testing.T) {
	generated := map[string]any{"Price": 9.99, "Quantity": 3}
	source := autoseed.NewSeededSource(1).Entity("OrderItem").Row(0).Field("Total")

	value := (inference.CorrelatedTotalRule{}).Infer(autoseed.Field{Name: "Total"}, source, generated).(float64)

	want := 29.97
	if value != want {
		t.Fatalf("Total = %v, want %v (Price 9.99 * Quantity 3)", value, want)
	}
}

func TestCorrelatedTotalRule_IndependentWithoutSiblings(t *testing.T) {
	source := autoseed.NewSeededSource(1).Entity("OrderItem").Row(0).Field("Total")
	value := (inference.CorrelatedTotalRule{}).Infer(autoseed.Field{Name: "Total"}, source, nil).(float64)
	if value < 1.00 || value > 999.99 {
		t.Fatalf("Total = %v, want a reasonable independent price-range value", value)
	}
}
