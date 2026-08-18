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
// same entity (a self-referencing many-to-many). JunctionParticipants is
// set the same way, only when entity's own primary key, or one of its
// UniqueConstraints, is exactly covered by the driver reference together
// with one or more other required references — a many-to-many join
// table, an explicit composite-key "attributed join" entity like an
// inventory or order-line row, or a ternary association with three or
// more participating references — where each driver row can pair with a
// given combination of JunctionParticipants rows at most once.
type EntityGenerationPlan struct {
	Entity               string
	RowCount             int
	Driver               string
	DriverFields         []string
	ChildCounts          []int
	JunctionParticipants []JunctionParticipant
}

// JunctionParticipant identifies one non-driver reference in a junction
// shape: a required reference that, together with the driver and zero or
// more sibling participants, exactly covers a composite key. Target and
// Fields work like Reference's own.
type JunctionParticipant struct {
	Target string
	Fields []string
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

		participants, maxPerDriverRow := junctionCap(byName[name], *driver, deferredEdges, rowCounts)
		if len(participants) == 0 && sharesDriverPrimaryKey(byName[name], *driver, deferredEdges) {
			maxPerDriverRow = 1
		}
		if len(participants) > 0 || maxPerDriverRow == 1 {
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
			Entity:               name,
			RowCount:             total,
			Driver:               driver.Target,
			DriverFields:         driver.Fields,
			ChildCounts:          childCounts,
			JunctionParticipants: participants,
		})
	}

	return &GenerationPlan{Entities: plans}, nil
}

// junctionCap reports the other required references that, together with
// driver, exactly account for every field of one of entity's composite
// keys — its own primary key, tried first, or one of its
// UniqueConstraints — the shape of a many-to-many join table, an
// explicit composite-key "attributed join" entity, or a ternary (or
// higher) association with three or more participating references.
// Matching is by Fields, not Target, so a self-referencing many-to-many
// (both references targeting the same entity as each other and as
// driver) is still recognized: the references are told apart by their
// own foreign key columns, never by target name alone. A further
// required reference elsewhere on the entity — one the driver's
// cardinality never needs to share the key with — does not disqualify a
// key that a different subset of references does cover; a key whose
// remaining fields cannot be covered by exactly one required reference
// per field, with no field claimed twice, is skipped in favor of the
// next candidate key.
//
// Each driver row can pair with a given combination of participant rows
// at most once, so a driver row's child count can never exceed the
// product of every participant's own usable row count: asking for more
// than that is asking for more distinct combinations than the
// participants can jointly supply, which PlanGeneration's caller then
// cannot assign without duplicating a combination and colliding on the
// key. A self-referencing participant lowers its own factor by one — a
// row can never pair with itself. Returns (nil, 0) when no candidate key
// is exactly covered this way.
func junctionCap(entity Entity, driver reference, deferredEdges map[string]bool, rowCounts map[string]int) ([]JunctionParticipant, int) {
	driverKey := strings.Join(driver.Fields, "+")
	driverFields := fieldSet(driver.Fields)

	for _, keyFields := range candidateKeyFieldSets(entity) {
		keySet := fieldSet(keyFields)
		if len(keySet) < 2 || !isSubset(keySet, driverFields) {
			continue
		}
		remaining := make(map[string]bool, len(keySet))
		for field := range keySet {
			if !driverFields[field] {
				remaining[field] = true
			}
		}
		if len(remaining) == 0 {
			continue
		}

		var participants []JunctionParticipant
		covered := make(map[string]bool, len(remaining))
		complete := true
		for _, ref := range entity.References {
			if ref.Nullable || deferredEdges[deferredEdgeKey(entity.Name, ref.Fields)] {
				continue
			}
			if strings.Join(ref.Fields, "+") == driverKey {
				continue
			}
			if !isSubset(remaining, fieldSet(ref.Fields)) {
				continue
			}

			overlap := false
			for _, field := range ref.Fields {
				if covered[field] {
					overlap = true
					break
				}
				covered[field] = true
			}
			if overlap {
				complete = false
				break
			}
			participants = append(participants, JunctionParticipant{Target: ref.Target, Fields: ref.Fields})
		}
		if !complete || len(covered) != len(remaining) {
			continue
		}

		sort.Slice(participants, func(i, j int) bool {
			if participants[i].Target != participants[j].Target {
				return participants[i].Target < participants[j].Target
			}
			return strings.Join(participants[i].Fields, "+") < strings.Join(participants[j].Fields, "+")
		})

		maxPerDriverRow := 1
		for _, p := range participants {
			base := rowCounts[p.Target]
			if p.Target == driver.Target {
				// Self-referencing: a row can never pair with itself, so
				// only N-1 of the N target rows are ever valid partners.
				// JunctionIndices guarantees this by construction (an
				// offset that skips the driver's own row), but the cap
				// itself must never promise N.
				base--
			}
			if base < 0 {
				base = 0
			}
			maxPerDriverRow *= base
		}
		return participants, maxPerDriverRow
	}

	return nil, 0
}

// candidateKeyFieldSets returns entity's own composite keys to try as a
// junction shape, in priority order: its primary key first (if it has
// one), then each UniqueConstraints entry in declared order.
func candidateKeyFieldSets(entity Entity) [][]string {
	var sets [][]string

	var pk []string
	for _, field := range entity.Fields {
		if field.PrimaryKey {
			pk = append(pk, field.Name)
		}
	}
	if len(pk) > 0 {
		sets = append(sets, pk)
	}

	return append(sets, entity.UniqueConstraints...)
}

func fieldSet(fields []string) map[string]bool {
	set := make(map[string]bool, len(fields))
	for _, field := range fields {
		set[field] = true
	}
	return set
}

func isSubset(super, sub map[string]bool) bool {
	for field := range sub {
		if !super[field] {
			return false
		}
	}
	return true
}

// JunctionIndices returns, for one row at position blockLocal within its
// driver row's block, the parent row index to pair with for each of
// participants, in the same order. rows reports how many rows already
// exist for a given entity name. driverRow is the driver's own row index
// for this block; a participant whose Target equals driverTarget is
// self-referencing and skips driverRow itself, since a row can never
// pair with itself.
//
// The mapping decomposes blockLocal in mixed radix over each
// participant's own usable row count, in participants' own order: this
// is injective as long as blockLocal is less than the product of those
// counts, exactly the guarantee PlanGeneration's cap on ChildCounts
// already gives every block. Two rows in the same driver block therefore
// always draw a distinct combination across every participant; two rows
// in different blocks differ by driverRow alone.
func JunctionIndices(participants []JunctionParticipant, driverTarget string, driverRow, blockLocal int, rows func(target string) int) []int {
	indices := make([]int, len(participants))
	remaining := blockLocal
	for i, p := range participants {
		full := rows(p.Target)
		selfReferencing := p.Target == driverTarget
		base := full
		if selfReferencing {
			base--
		}
		if base <= 0 {
			continue
		}

		digit := remaining % base
		remaining /= base
		if selfReferencing {
			indices[i] = (digit + driverRow + 1) % full
		} else {
			indices[i] = digit
		}
	}
	return indices
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
