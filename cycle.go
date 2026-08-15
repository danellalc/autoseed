package autoseed

import "sort"

// resolveCycles removes every cycle from edges. A cycle with at least one
// nullable edge is broken by deferring all of its nullable edges to a
// second pass; a cycle made entirely of required edges is unsatisfiable and
// returns ErrUnsatisfiableCycle naming every entity in it.
func resolveCycles(names []string, edges []graphEdge) (active []graphEdge, deferred []DeferredReference, err error) {
	active = append([]graphEdge(nil), edges...)

	for {
		components := stronglyConnectedComponents(names, active)
		foundCycle := false

		for _, component := range components {
			members := make(map[string]bool, len(component))
			for _, name := range component {
				members[name] = true
			}

			var internal []graphEdge
			for _, edge := range active {
				if members[edge.from] && members[edge.to] {
					internal = append(internal, edge)
				}
			}
			if len(internal) == 0 {
				continue
			}
			foundCycle = true

			var nullable []graphEdge
			for _, edge := range internal {
				if edge.nullable {
					nullable = append(nullable, edge)
				}
			}
			if len(nullable) == 0 {
				return nil, nil, unsatisfiableCycleError(component)
			}

			active = removeEdges(active, nullable)
			for _, edge := range nullable {
				deferred = append(deferred, DeferredReference{Entity: edge.from, Field: edge.field, Target: edge.to})
			}
		}

		if !foundCycle {
			break
		}
	}

	sort.Slice(deferred, func(i, j int) bool {
		if deferred[i].Entity != deferred[j].Entity {
			return deferred[i].Entity < deferred[j].Entity
		}
		return deferred[i].Field < deferred[j].Field
	})
	return active, deferred, nil
}

func removeEdges(edges, remove []graphEdge) []graphEdge {
	skip := make(map[graphEdge]bool, len(remove))
	for _, edge := range remove {
		skip[edge] = true
	}
	out := make([]graphEdge, 0, len(edges))
	for _, edge := range edges {
		if !skip[edge] {
			out = append(out, edge)
		}
	}
	return out
}

// stronglyConnectedComponents partitions names into strongly connected
// components using Tarjan's algorithm, processing nodes and each node's
// outgoing edges in sorted order so the result never depends on map
// iteration order. A component of size one with no self-loop is not a
// cycle; the caller decides that by checking for internal edges.
func stronglyConnectedComponents(names []string, edges []graphEdge) [][]string {
	adjacency := make(map[string][]string, len(names))
	for _, edge := range edges {
		adjacency[edge.from] = append(adjacency[edge.from], edge.to)
	}
	for from := range adjacency {
		sort.Strings(adjacency[from])
	}

	index := 0
	indices := make(map[string]int, len(names))
	lowlink := make(map[string]int, len(names))
	onStack := make(map[string]bool, len(names))
	var stack []string
	var components [][]string

	var connect func(v string)
	connect = func(v string) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adjacency[v] {
			if _, seen := indices[w]; !seen {
				connect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] && indices[w] < lowlink[v] {
				lowlink[v] = indices[w]
			}
		}

		if lowlink[v] == indices[v] {
			var component []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				component = append(component, w)
				if w == v {
					break
				}
			}
			sort.Strings(component)
			components = append(components, component)
		}
	}

	for _, name := range names {
		if _, seen := indices[name]; !seen {
			connect(name)
		}
	}

	sort.Slice(components, func(i, j int) bool { return components[i][0] < components[j][0] })
	return components
}
