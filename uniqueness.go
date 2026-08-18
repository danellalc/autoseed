package autoseed

import (
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
)

const maxUniquenessAttempts = 20

// EnsureUnique rewrites duplicate values, in row order, for every
// single-column string field on entity marked Unique, and for every
// composite tuple in entity.UniqueConstraints with at least one
// independent string column to rewrite. A constraint that names a
// foreign key field is left alone here entirely: at this stage a row's
// reference fields hold only a placeholder value, not the real parent
// key persistence assigns later, so there is nothing meaningful to
// deduplicate yet — that shape's uniqueness is guaranteed by construction
// in the generation plan instead. A constraint with no string field to
// rewrite is likewise out of scope, the same way a single non-string
// Unique field already is. The first row to use a value or tuple keeps
// it; a later duplicate has its rewritable field rewritten with a random
// numeric suffix, retried up to 20 times. Two constraints that share a
// rewritable field are resolved together, not independently: rewriting
// one row's field to satisfy one constraint can change what that same
// row's tuple looks like under every other constraint containing that
// field, so every affected constraint is re-checked until a row settles.
// seed must be scoped to the entity, not a specific row, and must not be
// nil. It returns ErrUnsatisfiableUniqueness, naming the entity and
// field(s), if a value cannot be fixed within the retry budget, or
// ErrNilSeed for a nil seed.
func EnsureUnique(entity Entity, rows []map[string]any, seed *SeededSource) error {
	if seed == nil {
		return ErrNilSeed
	}
	for _, field := range entity.Fields {
		if !field.Unique || field.Type == nil || field.Type.Kind() != reflect.String {
			continue
		}
		if err := ensureFieldUnique(rows, field, seed.Field(field.Name)); err != nil {
			return unsatisfiableUniquenessError(entity.Name, field.Name, maxUniquenessAttempts)
		}
	}

	referenceFields := referenceFieldNames(entity)
	var targets []constraintTarget
	for _, constraint := range entity.UniqueConstraints {
		field, ok := rewritableConstraintField(entity, constraint, referenceFields)
		if !ok {
			continue
		}
		targets = append(targets, constraintTarget{constraint: constraint, field: field})
	}
	if failed, err := ensureConstraintsUnique(rows, targets, seed); err != nil {
		return unsatisfiableUniquenessError(entity.Name, strings.Join(failed, "+"), maxUniquenessAttempts)
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

		unique, ok := makeUnique(value, field.Size, func(candidate string) bool { return seen[candidate] }, fieldSeed.Row(rowIndex))
		if !ok {
			return ErrUnsatisfiableUniqueness
		}
		row[field.Name] = unique
		seen[unique] = true
	}
	return nil
}

func referenceFieldNames(entity Entity) map[string]bool {
	names := make(map[string]bool)
	for _, ref := range entity.References {
		for _, field := range ref.Fields {
			names[field] = true
		}
	}
	return names
}

// rewritableConstraintField returns the last string-typed field in
// constraint — the one ensureConstraintUnique rewrites on a collision —
// and whether constraint is in scope at all: every field must be a known,
// non-reference Field, and at least one must be a string.
func rewritableConstraintField(entity Entity, constraint []string, referenceFields map[string]bool) (Field, bool) {
	byName := make(map[string]Field, len(entity.Fields))
	for _, field := range entity.Fields {
		byName[field.Name] = field
	}

	var target Field
	found := false
	for _, name := range constraint {
		if referenceFields[name] {
			return Field{}, false
		}
		field, ok := byName[name]
		if !ok {
			return Field{}, false
		}
		if field.Type != nil && field.Type.Kind() == reflect.String {
			target = field
			found = true
		}
	}
	return target, found
}

// constraintTarget pairs one composite constraint with the field
// rewritableConstraintField chose to rewrite for it.
type constraintTarget struct {
	constraint []string
	field      Field
}

// ensureConstraintsUnique rewrites duplicate tuples, in row order, across
// every target at once. Rewriting a row's field to satisfy one target can
// change what that row's tuple looks like under any OTHER target sharing
// that field, so every target is re-checked against the row's current
// values in a loop, per row, until a full pass makes no further change —
// resolving the shared field, not just the one target that first noticed
// the collision. Returns the constraint that could not be satisfied
// within the retry budget, and ErrUnsatisfiableUniqueness, on failure.
func ensureConstraintsUnique(rows []map[string]any, targets []constraintTarget, seed *SeededSource) ([]string, error) {
	if len(targets) == 0 {
		return nil, nil
	}

	seenByTarget := make([]map[string]bool, len(targets))
	for i := range seenByTarget {
		seenByTarget[i] = make(map[string]bool, len(rows))
	}

	for rowIndex, row := range rows {
		converged := false
		for pass := 0; pass < maxUniquenessAttempts; pass++ {
			changed := false
			for t, target := range targets {
				key, ok := constraintKey(row, target.constraint, "", nil)
				if !ok || !seenByTarget[t][key] {
					continue
				}

				value, ok := row[target.field.Name].(string)
				if !ok {
					return target.constraint, ErrUnsatisfiableUniqueness
				}

				isTaken := func(candidate string) bool {
					return anyTargetTaken(row, targets, target.field.Name, candidate, seenByTarget)
				}
				fieldSeed := seed.Field(strings.Join(target.constraint, "+")).Row(rowIndex).Row(pass)
				unique, ok := makeUnique(value, target.field.Size, isTaken, fieldSeed)
				if !ok {
					return target.constraint, ErrUnsatisfiableUniqueness
				}
				row[target.field.Name] = unique
				changed = true
			}
			if !changed {
				converged = true
				break
			}
		}
		if !converged {
			return targets[0].constraint, ErrUnsatisfiableUniqueness
		}

		for t, target := range targets {
			if key, ok := constraintKey(row, target.constraint, "", nil); ok {
				seenByTarget[t][key] = true
			}
		}
	}
	return nil, nil
}

// anyTargetTaken reports whether candidate, substituted for field in
// row, would collide with an earlier row under any target that includes
// field among its own constraint columns — not only the target currently
// being fixed, since two targets can share a field.
func anyTargetTaken(row map[string]any, targets []constraintTarget, field string, candidate any, seenByTarget []map[string]bool) bool {
	for t, target := range targets {
		if !constraintContainsField(target.constraint, field) {
			continue
		}
		key, ok := constraintKey(row, target.constraint, field, candidate)
		if ok && seenByTarget[t][key] {
			return true
		}
	}
	return false
}

func constraintContainsField(constraint []string, field string) bool {
	for _, name := range constraint {
		if name == field {
			return true
		}
	}
	return false
}

// constraintKey builds a comparable key for row's values across
// constraint, substituting overrideValue for overrideField when
// overrideField is non-empty — used to probe a candidate rewrite without
// mutating row before it is accepted.
func constraintKey(row map[string]any, constraint []string, overrideField string, overrideValue any) (string, bool) {
	parts := make([]string, len(constraint))
	for i, name := range constraint {
		value, ok := row[name]
		if overrideField != "" && name == overrideField {
			value, ok = overrideValue, true
		}
		if !ok {
			return "", false
		}
		parts[i] = fmt.Sprintf("%v", value)
	}
	return strings.Join(parts, "\x1f"), true
}

func makeUnique(value string, maxLength int, isTaken func(string) bool, attemptSeed *SeededSource) (string, bool) {
	for attempt := 0; attempt < maxUniquenessAttempts; attempt++ {
		candidate, ok := buildUniqueCandidate(value, maxLength, attemptSeed.Row(attempt).Rand())
		if ok && !isTaken(candidate) {
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
	prefixRunes := []rune(value)
	if len(prefixRunes) > availableForPrefix {
		prefixRunes = prefixRunes[:availableForPrefix]
	}
	return string(prefixRunes) + suffix, true
}
