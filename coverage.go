package autoseed

import (
	"reflect"
	"strings"
)

// PlanCoverage computes the smallest row count and cardinality decision
// per entity that exercises every axis SeedCoverage promises: a
// required, non-driver reference target never left at zero rows (the
// same backstop PlanGeneration itself relies on); a driver row hosting
// zero, one, and several ("many" — two, enough to prove the
// relationship isn't capped at one, without inflating row counts
// further) children for whatever depends on it; and, folded into
// whatever rows that already produces rather than adding rows of their
// own, a Nullable field, a bool-kind field, and a sized string field
// each cycling through their own boundary states across the entity's
// row set. It shares PlanGeneration's junction, shared-key and backstop
// rules unchanged through planWithStrategy — only how many rows a root
// gets, how many children a driver row gets, and the floor a dependent's
// own row count must reach, differ from the bulk, random-seed path: a
// dependent entity is itself floored at coverageRowCount too, through
// planWithStrategy's rowFloor hook, so an entity that is simultaneously a
// dependent (has a required reference of its own) and a driver target (or
// carries its own field axes) still gets enough rows for both roles,
// headroom under any shared-key or junction cap permitting. order and
// deferred come from DependencyGraph.Resolve().
func PlanCoverage(entities []Entity, order []string, deferred []DeferredReference) (*GenerationPlan, error) {
	deferredEdges := make(map[string]bool, len(deferred))
	for _, d := range deferred {
		deferredEdges[deferredEdgeKey(d.Entity, d.Fields)] = true
	}
	drivesADependent := driverTargets(entities, deferredEdges)
	byName := make(map[string]Entity, len(entities))
	for _, entity := range entities {
		byName[entity.Name] = entity
	}

	return planWithStrategy(entities, order, deferred, planStrategy{
		rootRowCount: func(entity Entity) int {
			return coverageRowCount(entity, drivesADependent[entity.Name])
		},
		childCounts: func(_ string, driverRows int) []int {
			return coverageChildCounts(driverRows)
		},
		rowFloor: func(name string) int {
			return coverageRowCount(byName[name], drivesADependent[name])
		},
	}), nil
}

// driverTargets returns the set of entity names some other entity picks
// as its driving principal — those need at least three rows under
// PlanCoverage, one to host each of a driver row's zero/one/many
// children, regardless of what a dependent's own field axes would
// otherwise ask for.
func driverTargets(entities []Entity, deferredEdges map[string]bool) map[string]bool {
	targets := make(map[string]bool)
	for _, entity := range entities {
		if driver := selectDriver(entity, deferredEdges); driver != nil {
			targets[driver.Target] = true
		}
	}
	return targets
}

func coverageRowCount(entity Entity, drivesADependent bool) int {
	count := 1
	if drivesADependent {
		count = 3
	}
	if axis := fieldAxisRowCount(entity); axis > count {
		count = axis
	}
	return count
}

// fieldAxisRowCount returns the row count needed to cycle every one of
// entity's own field-level coverage axes through all of their boundary
// states at least once: 2 for a Nullable or bool-kind field (its value
// left out entirely, and generated normally), 3 for a sized string field
// (empty, one character, and exactly Size characters).
func fieldAxisRowCount(entity Entity) int {
	need := 1
	for _, field := range entity.Fields {
		if field.Nullable && need < 2 {
			need = 2
		}
		if field.Type != nil && field.Type.Kind() == reflect.Bool && need < 2 {
			need = 2
		}
		if field.Type != nil && field.Type.Kind() == reflect.String && field.Size > 0 && need < 3 {
			need = 3
		}
	}
	return need
}

// coverageChildCounts cycles [0, 1, 2] across driverRows, so at least one
// driver row hosts zero children, one hosts exactly one, and one hosts
// two. Any cap PlanGeneration's own junction or shared-key clamping
// later applies (through the same planWithStrategy engine
// PlanGeneration itself runs through) still wins — a shared-primary-key
// one-to-one correctly degrades to zero/one/one, since "many" is
// structurally impossible for that shape regardless of which strategy
// proposed it.
func coverageChildCounts(driverRows int) []int {
	pattern := [3]int{0, 1, 2}
	counts := make([]int, driverRows)
	for i := range counts {
		counts[i] = pattern[i%len(pattern)]
	}
	return counts
}

// ApplyCoverageOverrides mutates rows in place so entity's own Nullable,
// bool-kind and sized string fields each cycle through their declared
// boundary states across the row set PlanCoverage already sized for
// this — a field never touches more than one axis, Nullable taking
// priority over bool-kind for a field that happens to be both, so a
// later axis's write can never silently undo an earlier one's. A
// reference's own foreign key field is skipped entirely: its real value
// is assigned separately, during persistence, and overwrites whatever a
// generated row held for it regardless — touching it here would only
// risk an adapter treating a placeholder null as if it meant something,
// for a field that will be overwritten before it ever reaches the
// database. A primary key field is skipped the same way: it is either
// database-generated or a natural key GenerateRow already owns, never a
// field whose boundary states are this feature's business. A Unique
// bool-kind field only has the override applied to its first two rows —
// enough to prove both true and false occur — since a bool has only two
// possible values, and forcing the same alternating pattern past a third
// row would manufacture a duplicate EnsureUnique cannot repair (it only
// rewrites string-kind fields); rows beyond the second keep whatever
// GenerateRow produced for them. GenerateRow's own value for a field no
// axis here claims is left untouched.
func ApplyCoverageOverrides(entity Entity, rows []map[string]any) {
	referenceFields := referenceFieldNames(entity)
	for _, field := range entity.Fields {
		if referenceFields[field.Name] || field.PrimaryKey {
			continue
		}
		switch {
		case field.Nullable:
			for i := range rows {
				if i%2 == 0 {
					rows[i][field.Name] = nil
				}
			}
		case field.Type != nil && field.Type.Kind() == reflect.Bool:
			for i := range rows {
				if field.Unique && i >= 2 {
					continue
				}
				rows[i][field.Name] = i%2 == 1
			}
		case field.Type != nil && field.Type.Kind() == reflect.String && field.Size > 0:
			for i := range rows {
				rows[i][field.Name] = coverageStringVariant(field.Size, i%3)
			}
		}
	}
}

// coverageStringVariant returns variant 0 (empty), 1 (one character), or
// 2 (exactly size characters) of a sized string field's boundary states.
// Variant 1 and variant 2 use different filler characters — not just the
// same one repeated — so the two boundary states are never the same
// string when size is 1 character: a Unique, one-character field would
// otherwise get an identical value on both its variant rows, a duplicate
// EnsureUnique's own retry has no room to rewrite (there is no character
// left for a distinguishing suffix once the whole field is one character
// long).
func coverageStringVariant(size, variant int) string {
	switch variant {
	case 0:
		return ""
	case 1:
		return "x"
	default:
		return strings.Repeat("y", size)
	}
}
