package autoseed

import (
	"fmt"
	"sort"
	"strings"
)

// SkipReason names one construct an adapter chose not to translate into the
// ModelSource contract: an entity, the field involved, and why. Explain
// reports these by name instead of an adapter silently generating wrong
// data for them.
type SkipReason struct {
	Entity string
	Field  string
	Reason string
}

// Plan is the result of resolving a ModelSource without writing anything:
// the insertion order, the references deferred to a second pass, and the
// constructs an adapter left out on purpose.
type Plan struct {
	Order    []string
	Deferred []DeferredReference
	Skipped  []SkipReason
}

// Explain resolves source into a Plan without writing to any database.
// skipped carries adapter-specific constructs — a polymorphic association,
// say — that source's Entities left out of its references on purpose;
// Explain folds them into the report instead of discarding them.
func Explain(source ModelSource, skipped ...SkipReason) (*Plan, error) {
	entities, err := source.Entities()
	if err != nil {
		return nil, fmt.Errorf("autoseed: reading model: %w", err)
	}

	graph, err := NewDependencyGraph(entities)
	if err != nil {
		return nil, err
	}

	result, err := graph.Resolve()
	if err != nil {
		return nil, err
	}

	sortedSkipped := append([]SkipReason(nil), skipped...)
	sort.Slice(sortedSkipped, func(i, j int) bool {
		if sortedSkipped[i].Entity != sortedSkipped[j].Entity {
			return sortedSkipped[i].Entity < sortedSkipped[j].Entity
		}
		return sortedSkipped[i].Field < sortedSkipped[j].Field
	})

	return &Plan{
		Order:    result.Order,
		Deferred: result.Deferred,
		Skipped:  sortedSkipped,
	}, nil
}

// Report renders p as a human-readable summary: insertion order, deferred
// references, then skipped constructs. A section with nothing to show is
// omitted.
func (p *Plan) Report() string {
	var b strings.Builder

	b.WriteString("Insertion order:\n")
	for i, name := range p.Order {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, name)
	}

	if len(p.Deferred) > 0 {
		b.WriteString("\nDeferred (second-pass) references:\n")
		for _, d := range p.Deferred {
			fmt.Fprintf(&b, "  %s.%s -> %s\n", d.Entity, strings.Join(d.Fields, "+"), d.Target)
		}
	}

	if len(p.Skipped) > 0 {
		b.WriteString("\nSkipped:\n")
		for _, s := range p.Skipped {
			fmt.Fprintf(&b, "  %s.%s: %s\n", s.Entity, s.Field, s.Reason)
		}
	}

	return b.String()
}
