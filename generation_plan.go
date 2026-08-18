package autoseed

import (
	"math"
	"math/rand/v2"
	"sort"
	"strings"
)

const defaultMeanChildrenPerParent = 3.0

// EntityGenerationPlan is the row count and cardinality decision for one
// entity: how many rows to generate and, for a dependent entity, how many
// of them belong to each row of its driving principal. JunctionTarget is
// set only when the entity's own primary key is exactly the driver's and
// this target's foreign key columns together — a many-to-many join table,
// or an explicit composite-key "attributed join" entity like an inventory
// or order-line row — where each driver row can pair with a given
// JunctionTarget row at most once.
type EntityGenerationPlan struct {
	Entity         string
	RowCount       int
	Driver         string
	ChildCounts    []int
	JunctionTarget string
}

// GenerationPlan is the row count and cardinality decision for every
// entity, in the same order Resolve produced.
type GenerationPlan struct {
	Entities []EntityGenerationPlan
}

// PlanGeneration computes a row count for every entity in order: a root
// entity — one with no required, non-deferred reference to another
// entity — gets options.Scale rows directly. A dependent entity picks a
// driving principal (its required reference whose Target sorts first;
// only one side of a many-to-many join table ever becomes a driver, by
// the same rule) and draws a child count per driver row from an
// Exponential(mean) distribution, long-tailed and occasionally zero, the
// same shape the .NET sibling uses. seed is the root SeededSource; order
// and deferred come from DependencyGraph.Resolve().
func PlanGeneration(entities []Entity, order []string, deferred []DeferredReference, seed *SeededSource, options Options) *GenerationPlan {
	byName := make(map[string]Entity, len(entities))
	for _, entity := range entities {
		byName[entity.Name] = entity
	}

	deferredEdges := make(map[string]bool, len(deferred))
	for _, d := range deferred {
		deferredEdges[deferredEdgeKey(d.Entity, d.Fields)] = true
	}
	requiredTargets := requiredReferenceTargets(entities, deferredEdges)

	rowCounts := make(map[string]int, len(order))
	plans := make([]EntityGenerationPlan, 0, len(order))
	planSeed := seed.Entity("GenerationPlan")

	for _, name := range order {
		driver := selectDriver(byName[name], deferredEdges)

		if driver == "" {
			rowCounts[name] = options.Scale
			plans = append(plans, EntityGenerationPlan{Entity: name, RowCount: options.Scale})
			continue
		}

		childCounts := drawChildCounts(planSeed.Entity(name), rowCounts[driver])

		junctionTarget, maxPerDriverRow := junctionCap(byName[name], driver, deferredEdges, rowCounts)
		if junctionTarget == "" && sharesDriverPrimaryKey(byName[name], driver, deferredEdges) {
			maxPerDriverRow = 1
		}
		if junctionTarget != "" || maxPerDriverRow == 1 {
			for i, count := range childCounts {
				if count > maxPerDriverRow {
					childCounts[i] = maxPerDriverRow
				}
			}
		}

		total := 0
		for _, count := range childCounts {
			total += count
		}

		if total == 0 && requiredTargets[name] && rowCounts[driver] > 0 {
			childCounts[0] = 1
			total = 1
		}

		rowCounts[name] = total
		plans = append(plans, EntityGenerationPlan{Entity: name, RowCount: total, Driver: driver, ChildCounts: childCounts, JunctionTarget: junctionTarget})
	}

	return &GenerationPlan{Entities: plans}
}

// junctionCap reports the target entity and row count of entity's one
// other required reference when, together with driver, it exactly
// accounts for every field of entity's own primary key — the shape of a
// many-to-many join table or an explicit composite-key "attributed join"
// entity. Each driver row can pair with a given target row at most once,
// so a driver row's child count can never exceed how many target rows
// exist: asking for more than that is asking for more distinct pairs
// than the target side can supply, which PlanGeneration's caller then
// cannot assign without duplicating a pair and colliding on the primary
// key. Returns ("", 0) for any other shape.
func junctionCap(entity Entity, driver string, deferredEdges map[string]bool, rowCounts map[string]int) (string, int) {
	var required []Reference
	for _, ref := range entity.References {
		if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
			continue
		}
		required = append(required, ref)
	}
	if len(required) != 2 {
		return "", 0
	}

	pkFields := make(map[string]bool)
	for _, field := range entity.Fields {
		if field.PrimaryKey {
			pkFields[field.Name] = true
		}
	}
	if len(pkFields) == 0 {
		return "", 0
	}

	target := ""
	refFields := make(map[string]bool)
	for _, ref := range required {
		if ref.Target != driver {
			if target != "" {
				return "", 0
			}
			target = ref.Target
		}
		for _, field := range ref.Fields {
			refFields[field] = true
		}
	}
	if target == "" || len(refFields) != len(pkFields) {
		return "", 0
	}
	for field := range refFields {
		if !pkFields[field] {
			return "", 0
		}
	}

	return target, rowCounts[target]
}

// sharesDriverPrimaryKey reports whether entity's only required reference
// is to driver and that reference's foreign key fields are exactly
// entity's own primary key — a shared-primary-key one-to-one, the GORM
// equivalent of EF Core's table splitting. A driver row's foreign key
// value becomes the child's own primary key verbatim, so two children for
// the same driver row would collide on it: at most one child per driver
// row, never a long-tail count.
func sharesDriverPrimaryKey(entity Entity, driver string, deferredEdges map[string]bool) bool {
	var required []Reference
	for _, ref := range entity.References {
		if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
			continue
		}
		required = append(required, ref)
	}
	if len(required) != 1 || required[0].Target != driver {
		return false
	}

	pkFields := make(map[string]bool)
	for _, field := range entity.Fields {
		if field.PrimaryKey {
			pkFields[field.Name] = true
		}
	}
	if len(pkFields) == 0 || len(pkFields) != len(required[0].Fields) {
		return false
	}
	for _, field := range required[0].Fields {
		if !pkFields[field] {
			return false
		}
	}
	return true
}

// requiredReferenceTargets returns the set of entity names that some other
// entity's required, non-deferred reference points at — including a
// non-driver principal, which selectDriver alone would never surface.
// Those entities cannot be left at zero rows without orphaning whatever
// depends on them.
func requiredReferenceTargets(entities []Entity, deferredEdges map[string]bool) map[string]bool {
	targets := make(map[string]bool)
	for _, entity := range entities {
		for _, ref := range entity.References {
			if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
				continue
			}
			targets[ref.Target] = true
		}
	}
	return targets
}

func selectDriver(entity Entity, deferredEdges map[string]bool) string {
	var candidates []string
	for _, ref := range entity.References {
		if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
			continue
		}
		candidates = append(candidates, ref.Target)
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	return candidates[0]
}

func deferredEdgeKey(entity string, fields []string) string {
	return entity + ">" + strings.Join(fields, "+")
}

func drawChildCounts(seed *SeededSource, driverRows int) []int {
	counts := make([]int, driverRows)
	for i := range counts {
		counts[i] = drawChildCount(seed.Row(i).Rand())
	}
	return counts
}

func drawChildCount(r *rand.Rand) int {
	uniform := r.Float64()
	drawn := -defaultMeanChildrenPerParent * math.Log(1-uniform)
	return int(math.Round(drawn))
}
