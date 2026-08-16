package inference

import (
	"math"

	"github.com/danellalc/autoseed"
)

const (
	minPrice = 1.00
	maxPrice = 999.99

	minQuantity = 1
	maxQuantity = 20
)

// PriceRule infers a reasonable monetary amount for a floating-point field
// ending in Price, Amount or Cost.
type PriceRule struct{}

// Priority runs PriceRule before CorrelatedTotalRule, which reads its
// output back.
func (PriceRule) Priority() int { return 0 }

// CanInfer matches a floating-point field ending in Price, Amount or Cost.
func (PriceRule) CanInfer(field autoseed.Field) bool {
	return isFloat(field.Type) && hasSuffix(field.Name, "Price", "Amount", "Cost")
}

// Infer generates an amount between 1.00 and 999.99.
func (PriceRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return faker(seed).Price(minPrice, maxPrice)
}

// QuantityRule infers a small positive count for an integer field ending
// in Quantity or Qty.
type QuantityRule struct{}

// Priority runs QuantityRule before CorrelatedTotalRule, which reads its
// output back.
func (QuantityRule) Priority() int { return 0 }

// CanInfer matches an integer field ending in Quantity or Qty.
func (QuantityRule) CanInfer(field autoseed.Field) bool {
	return isInt(field.Type) && hasSuffix(field.Name, "Quantity", "Qty")
}

// Infer generates a count between 1 and 20.
func (QuantityRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return faker(seed).Number(minQuantity, maxQuantity)
}

// CorrelatedTotalRule infers a floating-point field ending in Total or
// Subtotal as Price times Quantity when both were already generated on
// the same row, so the numbers actually add up, falling back to an
// independent price-range draw otherwise.
type CorrelatedTotalRule struct{}

// Priority runs CorrelatedTotalRule after PriceRule and QuantityRule, so a
// same-row Price and Quantity are already generated when Infer looks for
// them.
func (CorrelatedTotalRule) Priority() int { return 1 }

// CanInfer matches a floating-point field ending in Total or Subtotal.
func (CorrelatedTotalRule) CanInfer(field autoseed.Field) bool {
	return isFloat(field.Type) && hasSuffix(field.Name, "Total", "Subtotal")
}

// Infer generates Price times Quantity, floored to two decimal places,
// from a same-row Price and Quantity when both were already generated,
// otherwise an independent amount in PriceRule's own range.
func (CorrelatedTotalRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, generated map[string]any) any {
	price, hasPrice := findSibling[float64](generated, "Price")
	quantity, hasQuantity := findSibling[int](generated, "Quantity", "Qty")
	if hasPrice && hasQuantity {
		return math.Floor(price*float64(quantity)*100) / 100
	}
	return faker(seed).Price(minPrice, maxPrice)
}
