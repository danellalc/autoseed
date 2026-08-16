package inference_test

import (
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

// TestEmailRule_PrefersExactSiblingOverEarlierSortingSuffixMatch guards a
// real bug: findSibling used to return the alphabetically-first key ending
// in a suffix, with no preference for an exact match. "BillingFirstName"
// sorts before "FirstName", so EmailRule silently built its address from
// the wrong field.
func TestEmailRule_PrefersExactSiblingOverEarlierSortingSuffixMatch(t *testing.T) {
	generated := map[string]any{
		"BillingFirstName": "Cali",
		"FirstName":        "Ada",
		"LastName":         "Lovelace",
	}
	source := autoseed.NewSeededSource(1).Entity("Customer").Row(0).Field("Email")

	value := (inference.EmailRule{}).Infer(autoseed.Field{Name: "Email"}, source, generated).(string)

	want := "ada.lovelace@"
	if len(value) < len(want) || value[:len(want)] != want {
		t.Fatalf("Email = %q, want it to start with %q (the row's own FirstName/LastName, not BillingFirstName)", value, want)
	}
}

// TestCorrelatedTotalRule_PrefersExactSiblingOverEarlierSortingSuffixMatch
// covers the same findSibling bug for Price: "ListPrice" sorts before
// "Price" and must not be picked over the field actually named Price.
func TestCorrelatedTotalRule_PrefersExactSiblingOverEarlierSortingSuffixMatch(t *testing.T) {
	generated := map[string]any{
		"ListPrice": 436.78,
		"Price":     10.00,
		"Quantity":  2,
	}
	source := autoseed.NewSeededSource(1).Entity("OrderItem").Row(0).Field("Total")

	value := (inference.CorrelatedTotalRule{}).Infer(autoseed.Field{Name: "Total"}, source, generated).(float64)

	want := 20.00
	if value != want {
		t.Fatalf("Total = %v, want %v (Price 10.00 * Quantity 2, not ListPrice)", value, want)
	}
}

// TestUpdatedAtRule_PrefersExactSiblingOverEarlierSortingSuffixMatch covers
// the same findSibling bug for CreatedAt: "ArchivedCreatedAt" sorts before
// "CreatedAt" and must not be picked as the anchor.
func TestUpdatedAtRule_PrefersExactSiblingOverEarlierSortingSuffixMatch(t *testing.T) {
	archived := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	generated := map[string]any{
		"ArchivedCreatedAt": archived,
		"CreatedAt":         createdAt,
	}

	for seed := uint64(0); seed < 20; seed++ {
		source := autoseed.NewSeededSource(seed).Entity("E").Row(0).Field("UpdatedAt")
		value := (inference.UpdatedAtRule{}).Infer(autoseed.Field{Name: "UpdatedAt"}, source, generated).(time.Time)
		if value.Before(createdAt) {
			t.Fatalf("seed %d: UpdatedAt = %v is before the row's own CreatedAt = %v (anchored to ArchivedCreatedAt instead)", seed, value, createdAt)
		}
	}
}
