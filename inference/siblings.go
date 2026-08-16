package inference

import (
	"sort"
	"strings"
)

func findSibling[T any](generated map[string]any, suffixes ...string) (T, bool) {
	var zero T

	keys := make([]string, 0, len(generated))
	for key := range generated {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if !isExactMatch(key, suffixes...) {
			continue
		}
		if value, ok := generated[key].(T); ok {
			return value, true
		}
	}
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

func isExactMatch(name string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(name, candidate) {
			return true
		}
	}
	return false
}
