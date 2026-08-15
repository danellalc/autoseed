package autoseed_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/danellalc/autoseed"
	"pgregory.net/rapid"
)

// TestResolve_RespectsDependencies checks the property the whole engine
// exists for: for any model and any mix of nullable/required references,
// Resolve either names an unsatisfiable cycle or returns an order where
// every non-deferred reference's target comes before its source.
func TestResolve_RespectsDependencies(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 6).Draw(t, "n")
		names := make([]string, n)
		for i := range names {
			names[i] = fmt.Sprintf("E%d", i)
		}

		type ref struct {
			from, to, field string
			nullable        bool
		}
		var refs []ref
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if !rapid.Bool().Draw(t, fmt.Sprintf("edge-%d-%d", i, j)) {
					continue
				}
				refs = append(refs, ref{
					from:     names[i],
					to:       names[j],
					field:    fmt.Sprintf("F%d_%d", i, j),
					nullable: rapid.Bool().Draw(t, fmt.Sprintf("nullable-%d-%d", i, j)),
				})
			}
		}

		byEntity := make(map[string][]autoseed.Reference, n)
		for _, r := range refs {
			byEntity[r.from] = append(byEntity[r.from], autoseed.Reference{
				Name: r.field, Target: r.to, Nullable: r.nullable,
			})
		}
		entities := make([]autoseed.Entity, n)
		for i, name := range names {
			entities[i] = autoseed.Entity{Name: name, References: byEntity[name]}
		}

		graph, err := autoseed.NewDependencyGraph(entities)
		if err != nil {
			t.Fatalf("NewDependencyGraph: %v", err)
		}
		result, err := graph.Resolve()
		if err != nil {
			if !errors.Is(err, autoseed.ErrUnsatisfiableCycle) {
				t.Fatalf("unexpected error: %v", err)
			}
			return
		}

		if len(result.Order) != n {
			t.Fatalf("Order has %d entries, want %d", len(result.Order), n)
		}
		position := make(map[string]int, n)
		for i, name := range result.Order {
			position[name] = i
		}

		deferred := make(map[[3]string]bool, len(result.Deferred))
		for _, d := range result.Deferred {
			deferred[[3]string{d.Entity, d.Field, d.Target}] = true
		}

		for _, r := range refs {
			if deferred[[3]string{r.from, r.field, r.to}] {
				continue
			}
			if r.from == r.to {
				t.Fatalf("self-reference %s.%s survived resolution undeferred", r.from, r.field)
			}
			if position[r.to] >= position[r.from] {
				t.Fatalf("%s must come before %s (via %s.%s), got positions %d, %d",
					r.to, r.from, r.from, r.field, position[r.to], position[r.from])
			}
		}
	})
}
