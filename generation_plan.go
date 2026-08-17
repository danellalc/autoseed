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
// of them belong to each row of its driving principal.
type EntityGenerationPlan struct {
	Entity      string
	RowCount    int
	Driver      string
	ChildCounts []int
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
		total := 0
		for _, count := range childCounts {
			total += count
		}
		rowCounts[name] = total
		plans = append(plans, EntityGenerationPlan{Entity: name, RowCount: total, Driver: driver, ChildCounts: childCounts})
	}

	return &GenerationPlan{Entities: plans}
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
