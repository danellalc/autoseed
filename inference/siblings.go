package inference

import "sort"

func findSibling[T any](generated map[string]any, suffixes ...string) (T, bool) {
	var zero T

	keys := make([]string, 0, len(generated))
	for key := range generated {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if !hasSuffix(key, suffixes...) {
			continue
		}
		if value, ok := generated[key].(T); ok {
			return value, true
		}
	}
	return zero, false
}
