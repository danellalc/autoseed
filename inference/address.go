package inference

import (
	"reflect"

	"github.com/danellalc/autoseed"
)

// AddressRule infers a street name or city for a string field ending in
// Street, StreetName, Address or City.
type AddressRule struct{}

// Priority runs AddressRule before generic string rules.
func (AddressRule) Priority() int { return 0 }

// CanInfer matches a string field ending in Street, StreetName, Address or
// City.
func (AddressRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "Street", "StreetName", "Address", "City")
}

// Infer generates a city name for a field ending in City, a street name
// otherwise.
func (AddressRule) Infer(field autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	f := faker(seed)
	if hasSuffix(field.Name, "City") {
		return f.City()
	}
	return f.StreetName()
}
