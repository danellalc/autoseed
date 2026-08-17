package autoseed

import (
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
)

const maxUniquenessAttempts = 20

// EnsureUnique rewrites duplicate values, in row order, for every
// single-column string field on entity marked Unique — composite unique
// constraints and non-string columns are not covered. The first row to
// use a value keeps it; a later duplicate is rewritten with a random
// numeric suffix, retried up to 20 times. seed must be scoped to the
// entity, not a specific row. It returns ErrUnsatisfiableUniqueness,
// naming the entity and field, if a value cannot be fixed within the
// retry budget.
func EnsureUnique(entity Entity, rows []map[string]any, seed *SeededSource) error {
	for _, field := range entity.Fields {
		if !field.Unique || field.Type == nil || field.Type.Kind() != reflect.String {
			continue
		}
		if err := ensureFieldUnique(rows, field, seed.Field(field.Name)); err != nil {
			return unsatisfiableUniquenessError(entity.Name, field.Name, maxUniquenessAttempts)
		}
	}
	return nil
}

func ensureFieldUnique(rows []map[string]any, field Field, fieldSeed *SeededSource) error {
	seen := make(map[string]bool, len(rows))

	for rowIndex, row := range rows {
		value, ok := row[field.Name].(string)
		if !ok {
			continue
		}
		if !seen[value] {
			seen[value] = true
			continue
		}

		unique, ok := makeUnique(value, field.Size, seen, fieldSeed.Row(rowIndex))
		if !ok {
			return ErrUnsatisfiableUniqueness
		}
		row[field.Name] = unique
		seen[unique] = true
	}
	return nil
}

func makeUnique(value string, maxLength int, seen map[string]bool, attemptSeed *SeededSource) (string, bool) {
	for attempt := 0; attempt < maxUniquenessAttempts; attempt++ {
		candidate, ok := buildUniqueCandidate(value, maxLength, attemptSeed.Row(attempt).Rand())
		if ok && !seen[candidate] {
			return candidate, true
		}
	}
	return "", false
}

func buildUniqueCandidate(value string, maxLength int, r *rand.Rand) (string, bool) {
	if maxLength <= 0 {
		return fmt.Sprintf("%s-%d", value, r.IntN(1_000_000_000)), true
	}

	maxSuffixDigits := maxLength - 1
	if maxSuffixDigits > 9 {
		maxSuffixDigits = 9
	}
	if maxSuffixDigits < 1 {
		return "", false
	}

	bound := int(math.Pow10(maxSuffixDigits))
	suffix := fmt.Sprintf("-%d", r.IntN(bound))

	availableForPrefix := maxLength - len(suffix)
	if availableForPrefix < 0 {
		availableForPrefix = 0
	}
	prefix := value
	if len(prefix) > availableForPrefix {
		prefix = prefix[:availableForPrefix]
	}
	return prefix + suffix, true
}
