package inference

import (
	"reflect"

	"github.com/danellalc/autoseed"
)

// URLRule infers a URL for a string field ending in URL, URI or Website.
type URLRule struct{}

// Priority runs URLRule before generic string rules.
func (URLRule) Priority() int { return 0 }

// CanInfer matches a string field ending in URL, URI or Website.
func (URLRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "URL", "URI", "Website")
}

// Infer generates a URL.
func (URLRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return faker(seed).URL()
}
