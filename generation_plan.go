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
// of them belong to each row of its driving principal. Driver and
// DriverFields together identify the specific reference chosen as the
// driver — Fields disambiguates the case where two references target the
// same entity (a self-referencing many-to-many). JunctionTarget and
// JunctionFields are set the same way, only when the entity's own primary
// key is exactly the driver reference's and one other required
// reference's foreign key columns together — a many-to-many join table,
// or an explicit composite-key "attributed join" entity like an inventory
// or order-line row — where each driver row can pair with a given
// JunctionTarget row at most once.
type EntityGenerationPlan struct {
	Entity         string
	RowCount       int
	Driver         string
	DriverFields   []string
	ChildCounts    []int
	JunctionTarget string
	JunctionFields []string
}

// GenerationPlan is the row count and cardinality decision for every
// entity, in the same order Resolve produced.
type GenerationPlan struct {
	Entities []EntityGenerationPlan
}

// PlanGeneration computes a row count for every entity in order: a root
// entity — one with no required, non-deferred reference to another
// entity — gets options.Scale rows directly. A dependent entity picks a
// driving principal (its required reference whose Target sorts first,
// Fields breaking a tie between two references to the same target; only
// one side of a many-to-many join table ever becomes a driver, by the
// same rule) and draws a child count per driver row from an
// Exponential(mean) distribution, long-tailed and occasionally zero, the
// same shape the .NET sibling uses. seed is the root SeededSource; order
// and deferred come from DependencyGraph.Resolve(). It returns
// ErrNilSeed for a nil seed and ErrInvalidScale for a negative
// options.Scale.
func PlanGeneration(entities []Entity, order []string, deferred []DeferredReference, seed *SeededSource, options Options) (*GenerationPlan, error) {
	if seed == nil {
		return nil, ErrNilSeed
	}
	if options.Scale < 0 {
		return nil, invalidScaleError(options.Scale)
	}

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

		if driver == nil {
			rowCounts[name] = options.Scale
			plans = append(plans, EntityGenerationPlan{Entity: name, RowCount: options.Scale})
			continue
		}

		childCounts := drawChildCounts(planSeed.Entity(name), rowCounts[driver.Target])

		junctionTarget, junctionFields, maxPerDriverRow := junctionCap(byName[name], *driver, deferredEdges, rowCounts)
		if junctionTarget == "" && sharesDriverPrimaryKey(byName[name], *driver, deferredEdges) {
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

		if total == 0 && requiredTargets[name] && rowCounts[driver.Target] > 0 {
			childCounts[0] = 1
			total = 1
		}

		rowCounts[name] = total
		plans = append(plans, EntityGenerationPlan{
			Entity:         name,
			RowCount:       total,
			Driver:         driver.Target,
			DriverFields:   driver.Fields,
			ChildCounts:    childCounts,
			JunctionTarget: junctionTarget,
			JunctionFields: junctionFields,
		})
	}

	return &GenerationPlan{Entities: plans}, nil
}

// junctionCap reports the target, fields and row count of the one other
// required reference that, together with driver, exactly accounts for
// every field of entity's own primary key — the shape of a many-to-many
// join table or an explicit composite-key "attributed join" entity.
// Matching is by Fields, not Target, so a self-referencing many-to-many
// (both references targeting the same entity as each other and as
// driver) is still recognized: the two references are told apart by
// their own foreign key columns, never by target name alone. A third
// required reference elsewhere on the entity — one driver's cardinality
// never needs to share the primary key with — does not disqualify the
// pair that does cover it. Each driver row can pair with a given target
// row at most once, so a driver row's child count can never exceed how
// many target rows exist: asking for more than that is asking for more
// distinct pairs than the target side can supply, which PlanGeneration's
// caller then cannot assign without duplicating a pair and colliding on
// the primary key. Self-referencing lowers that ceiling by one — a row
// can never pair with itself. Returns ("", nil, 0) when no other
// required reference exactly completes the primary key together with
// driver — including a primary key of three or more foreign key columns,
// out of scope for this pairwise check.
func junctionCap(entity Entity, driver reference, deferredEdges map[string]bool, rowCounts map[string]int) (string, []string, int) {
	pkFields := make(map[string]bool)
	for _, field := range entity.Fields {
		if field.PrimaryKey {
			pkFields[field.Name] = true
		}
	}
	if len(pkFields) == 0 {
		return "", nil, 0
	}

	driverKey := strings.Join(driver.Fields, "+")
	driverFields := make(map[string]bool, len(driver.Fields))
	for _, field := range driver.Fields {
		driverFields[field] = true
	}

	for _, ref := range entity.References {
		if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
			continue
		}
		if strings.Join(ref.Fields, "+") == driverKey {
			continue
		}

		combined := make(map[string]bool, len(pkFields))
		for field := range driverFields {
			combined[field] = true
		}
		for _, field := range ref.Fields {
			combined[field] = true
		}
		if len(combined) != len(pkFields) {
			continue
		}
		match := true
		for field := range combined {
			if !pkFields[field] {
				match = false
				break
			}
		}
		if match {
			maxPerDriverRow := rowCounts[ref.Target]
			if ref.Target == driver.Target {
				// Self-referencing: a row can never pair with itself, so
				// only N-1 of the N target rows are ever valid partners.
				// persist.go's junction assignment guarantees this by
				// construction (an offset that skips the driver's own
				// row), but the cap itself must never promise N.
				maxPerDriverRow--
				if maxPerDriverRow < 0 {
					maxPerDriverRow = 0
				}
			}
			return ref.Target, ref.Fields, maxPerDriverRow
		}
	}

	return "", nil, 0
}

// sharesDriverPrimaryKey reports whether entity's only required reference
// is the driver reference and its foreign key fields are exactly
// entity's own primary key — a shared-primary-key one-to-one, the GORM
// equivalent of EF Core's table splitting. A driver row's foreign key
// value becomes the child's own primary key verbatim, so two children for
// the same driver row would collide on it: at most one child per driver
// row, never a long-tail count.
func sharesDriverPrimaryKey(entity Entity, driver reference, deferredEdges map[string]bool) bool {
	var required []Reference
	for _, ref := range entity.References {
		if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
			continue
		}
		required = append(required, ref)
	}
	if len(required) != 1 || strings.Join(required[0].Fields, "+") != strings.Join(driver.Fields, "+") {
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

// reference identifies one specific required reference on an entity: its
// target and its own foreign key field set. Two references can share a
// Target (a self-referencing association); Fields never collide, since
// each reference owns a distinct set of foreign key columns.
type reference struct {
	Target string
	Fields []string
}

// selectDriver picks entity's driving reference: the required,
// non-deferred reference whose Target sorts first, Fields (joined)
// breaking a tie when two references share a Target. Returns nil if
// entity has no such reference.
func selectDriver(entity Entity, deferredEdges map[string]bool) *reference {
	var candidates []reference
	for _, ref := range entity.References {
		if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
			continue
		}
		candidates = append(candidates, reference{Target: ref.Target, Fields: ref.Fields})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Target != candidates[j].Target {
			return candidates[i].Target < candidates[j].Target
		}
		return strings.Join(candidates[i].Fields, "+") < strings.Join(candidates[j].Fields, "+")
	})
	return &candidates[0]
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
