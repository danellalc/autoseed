package autoseed

import (
	"sort"
	"strings"
)

// resolveCycles breaks every cycle in edges by deferring the single
// nullable edge that breaks it — never every nullable edge in the
// component at once, which would defer FKs a cycle never needed broken
// and understate their driving entity's real cardinality. Candidates tie
// broken by (from, to, fields), same order Explain reports insertion in.
// A component with no nullable edge at all is unsatisfiable; every such
// component is collected before returning, so one Resolve call names
// every independent unsatisfiable cycle, not just the first found.
func resolveCycles(names []string, edges []graphEdge) ([]graphEdge, []DeferredReference, error) {
	active := append([]graphEdge(nil), edges...)
	var deferred []DeferredReference
	var unsatisfiable [][]string

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

			nullable := dedupeEdges(filterNullable(internal))
			if len(nullable) == 0 {
				unsatisfiable = append(unsatisfiable, findCycle(component, internal))
				active = removeEdges(active, internal)
				continue
			}

			chosen := minEdge(nullable)
			active = removeEdges(active, []graphEdge{chosen})
			deferred = append(deferred, DeferredReference{Entity: chosen.from, Fields: chosen.fields, Target: chosen.to})
		}

		if !foundCycle {
			break
		}
	}

	if len(unsatisfiable) > 0 {
		sort.Slice(unsatisfiable, func(i, j int) bool {
			return strings.Join(unsatisfiable[i], ">") < strings.Join(unsatisfiable[j], ">")
		})
		return nil, nil, unsatisfiableCycleError(unsatisfiable)
	}

	sort.Slice(deferred, func(i, j int) bool {
		if deferred[i].Entity != deferred[j].Entity {
			return deferred[i].Entity < deferred[j].Entity
		}
		return strings.Join(deferred[i].Fields, "+") < strings.Join(deferred[j].Fields, "+")
	})
	return active, deferred, nil
}

func minEdge(edges []graphEdge) graphEdge {
	chosen := edges[0]
	for _, edge := range edges[1:] {
		if edge.from != chosen.from {
			if edge.from < chosen.from {
				chosen = edge
			}
			continue
		}
		if edge.to != chosen.to {
			if edge.to < chosen.to {
				chosen = edge
			}
			continue
		}
		if strings.Join(edge.fields, "+") < strings.Join(chosen.fields, "+") {
			chosen = edge
		}
	}
	return chosen
}

func filterNullable(edges []graphEdge) []graphEdge {
	var nullable []graphEdge
	for _, edge := range edges {
		if edge.nullable {
			nullable = append(nullable, edge)
		}
	}
	return nullable
}

func dedupeEdges(edges []graphEdge) []graphEdge {
	seen := make(map[string]bool, len(edges))
	out := make([]graphEdge, 0, len(edges))
	for _, edge := range edges {
		key := edgeKey(edge)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, edge)
	}
	return out
}

func removeEdges(edges, remove []graphEdge) []graphEdge {
	skip := make(map[string]bool, len(remove))
	for _, edge := range remove {
		skip[edgeKey(edge)] = true
	}
	out := make([]graphEdge, 0, len(edges))
	for _, edge := range edges {
		if !skip[edgeKey(edge)] {
			out = append(out, edge)
		}
	}
	return out
}

func edgeKey(edge graphEdge) string {
	return edge.from + ">" + edge.to + ">" + strings.Join(edge.fields, "+")
}

func findCycle(component []string, edges []graphEdge) []string {
	adjacency := make(map[string][]string, len(component))
	for _, edge := range edges {
		adjacency[edge.from] = append(adjacency[edge.from], edge.to)
	}
	for from := range adjacency {
		sort.Strings(adjacency[from])
	}

	onPath := make(map[string]int, len(component))
	done := make(map[string]bool, len(component))
	var path []string

	var walk func(v string) []string
	walk = func(v string) []string {
		onPath[v] = len(path)
		path = append(path, v)

		for _, w := range adjacency[v] {
			if idx, active := onPath[w]; active {
				cycle := append([]string(nil), path[idx:]...)
				return append(cycle, w)
			}
			if done[w] {
				continue
			}
			if found := walk(w); found != nil {
				return found
			}
		}

		path = path[:len(path)-1]
		delete(onPath, v)
		done[v] = true
		return nil
	}

	if cycle := walk(component[0]); cycle != nil {
		return cycle
	}
	return component
}

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
