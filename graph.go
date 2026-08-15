package autoseed

import "sort"

// graphEdge is one foreign key reference in the dependency graph: from
// depends on to, meaning a row of from cannot be inserted before a row of
// to exists.
type graphEdge struct {
	from     string
	to       string
	field    string
	nullable bool
}

// DependencyGraph is the directed graph of foreign key references between
// an ORM model's entities, built from a ModelSource's Entities.
type DependencyGraph struct {
	names []string
	edges []graphEdge
}

// NewDependencyGraph builds a DependencyGraph from entities. It returns
// ErrUnknownReference if any Reference.Target does not match an Entity.Name
// in entities.
func NewDependencyGraph(entities []Entity) (*DependencyGraph, error) {
	names := make([]string, 0, len(entities))
	known := make(map[string]bool, len(entities))
	for _, entity := range entities {
		names = append(names, entity.Name)
		known[entity.Name] = true
	}
	sort.Strings(names)

	var edges []graphEdge
	for _, entity := range entities {
		for _, ref := range entity.References {
			if !known[ref.Target] {
				return nil, unknownReferenceError(entity.Name, ref.Name, ref.Target)
			}
			edges = append(edges, graphEdge{
				from:     entity.Name,
				to:       ref.Target,
				field:    ref.Name,
				nullable: ref.Nullable,
			})
		}
	}
	sortEdges(edges)

	return &DependencyGraph{names: names, edges: edges}, nil
}

// TopologicalSortResult is the outcome of resolving a DependencyGraph: the
// insertion order and the references that could not be satisfied on first
// insert and must be patched in a second pass.
type TopologicalSortResult struct {
	Order    []string
	Deferred []DeferredReference
}

// DeferredReference is a nullable reference that participates in a cycle:
// it must be inserted as null and patched to its real value once every
// entity in Order has been written.
type DeferredReference struct {
	Entity string
	Field  string
	Target string
}

// Resolve resolves every cycle in the graph and returns a stable
// topological order for the remaining, acyclic references. Ties in the
// order break on entity name, never on map iteration order, so the same
// graph always resolves to the same order. It returns ErrUnsatisfiableCycle
// if a cycle has no nullable reference to break it.
func (g *DependencyGraph) Resolve() (*TopologicalSortResult, error) {
	active, deferred, err := resolveCycles(g.names, g.edges)
	if err != nil {
		return nil, err
	}
	return &TopologicalSortResult{
		Order:    stableTopologicalSort(g.names, active),
		Deferred: deferred,
	}, nil
}

// stableTopologicalSort orders names so that, for every edge from->to in
// edges, to appears before from. edges must already be acyclic. Among
// names with no remaining unresolved dependency, the lexicographically
// smallest is placed next, so the result never depends on map or slice
// iteration order.
func stableTopologicalSort(names []string, edges []graphEdge) []string {
	dependencies := make(map[string]map[string]bool, len(names))
	dependents := make(map[string][]string, len(names))
	for _, name := range names {
		dependencies[name] = map[string]bool{}
	}
	for _, edge := range edges {
		if edge.from == edge.to {
			continue
		}
		if !dependencies[edge.from][edge.to] {
			dependencies[edge.from][edge.to] = true
			dependents[edge.to] = append(dependents[edge.to], edge.from)
		}
	}

	ready := make([]string, 0, len(names))
	for _, name := range names {
		if len(dependencies[name]) == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)

	order := make([]string, 0, len(names))
	for len(ready) > 0 {
		next := ready[0]
		ready = ready[1:]
		order = append(order, next)

		newlyReady := make([]string, 0)
		for _, dependent := range sortedCopy(dependents[next]) {
			delete(dependencies[dependent], next)
			if len(dependencies[dependent]) == 0 {
				newlyReady = append(newlyReady, dependent)
			}
		}
		ready = mergeSorted(ready, newlyReady)
	}
	return order
}

func sortedCopy(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	sort.Strings(out)
	return out
}

func mergeSorted(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	sort.Strings(b)
	merged := make([]string, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] <= b[j] {
			merged = append(merged, a[i])
			i++
		} else {
			merged = append(merged, b[j])
			j++
		}
	}
	merged = append(merged, a[i:]...)
	merged = append(merged, b[j:]...)
	return merged
}

func sortEdges(edges []graphEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		if edges[i].to != edges[j].to {
			return edges[i].to < edges[j].to
		}
		return edges[i].field < edges[j].field
	})
}
