package autoseed_test

import (
	"errors"
	"testing"

	"github.com/danellalc/autoseed"
)

func TestResolve_NullableSelfReference(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Employee",
			References: []autoseed.Reference{
				{Name: "ManagerID", Target: "Employee", Nullable: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	result, err := graph.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !equalStrings(result.Order, []string{"Employee"}) {
		t.Fatalf("Order = %v, want [Employee]", result.Order)
	}
	want := []autoseed.DeferredReference{{Entity: "Employee", Field: "ManagerID", Target: "Employee"}}
	if !equalDeferred(result.Deferred, want) {
		t.Fatalf("Deferred = %v, want %v", result.Deferred, want)
	}
}

func TestResolve_RequiredSelfReference(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Employee",
			References: []autoseed.Reference{
				{Name: "ManagerID", Target: "Employee", Nullable: false},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	_, err = graph.Resolve()
	if !errors.Is(err, autoseed.ErrUnsatisfiableCycle) {
		t.Fatalf("got %v, want ErrUnsatisfiableCycle", err)
	}
}

func TestResolve_NullableCrossEntityCycle(t *testing.T) {
	// Order.ContactID -> Contact, Contact.DefaultOrderID -> Order, the
	// second link nullable so the cycle is resolvable in a second pass.
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Order",
			References: []autoseed.Reference{
				{Name: "ContactID", Target: "Contact", Nullable: false},
			},
		},
		{
			Name: "Contact",
			References: []autoseed.Reference{
				{Name: "DefaultOrderID", Target: "Order", Nullable: true},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	result, err := graph.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Order.ContactID is required, so Contact (the dependency) must be
	// inserted first; Contact.DefaultOrderID is the deferred link back.
	if !equalStrings(result.Order, []string{"Contact", "Order"}) {
		t.Fatalf("Order = %v, want [Contact Order]", result.Order)
	}
	want := []autoseed.DeferredReference{{Entity: "Contact", Field: "DefaultOrderID", Target: "Order"}}
	if !equalDeferred(result.Deferred, want) {
		t.Fatalf("Deferred = %v, want %v", result.Deferred, want)
	}
}

func TestResolve_RequiredCrossEntityCycle(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Order",
			References: []autoseed.Reference{
				{Name: "ContactID", Target: "Contact", Nullable: false},
			},
		},
		{
			Name: "Contact",
			References: []autoseed.Reference{
				{Name: "DefaultOrderID", Target: "Order", Nullable: false},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	_, err = graph.Resolve()
	if !errors.Is(err, autoseed.ErrUnsatisfiableCycle) {
		t.Fatalf("got %v, want ErrUnsatisfiableCycle", err)
	}
}

// TestResolve_StableAcrossRuns guards the #1 determinism trap in this
// codebase: a map iterated somewhere in the resolution path would make this
// flaky. It is not.
func TestResolve_StableAcrossRuns(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Zebra", References: []autoseed.Reference{{Name: "RootID", Target: "Root"}}},
		{Name: "Mango", References: []autoseed.Reference{{Name: "RootID", Target: "Root"}}},
		{Name: "Apple", References: []autoseed.Reference{{Name: "RootID", Target: "Root"}}},
		{Name: "Root"},
		{
			Name: "Loop",
			References: []autoseed.Reference{
				{Name: "PeerID", Target: "Peer", Nullable: true},
			},
		},
		{
			Name: "Peer",
			References: []autoseed.Reference{
				{Name: "LoopID", Target: "Loop", Nullable: true},
			},
		},
	}

	graph, err := autoseed.NewDependencyGraph(entities)
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}
	first, err := graph.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	for i := 0; i < 100; i++ {
		graph, err := autoseed.NewDependencyGraph(entities)
		if err != nil {
			t.Fatalf("NewDependencyGraph (run %d): %v", i, err)
		}
		result, err := graph.Resolve()
		if err != nil {
			t.Fatalf("Resolve (run %d): %v", i, err)
		}
		if !equalStrings(result.Order, first.Order) {
			t.Fatalf("run %d: Order = %v, want %v", i, result.Order, first.Order)
		}
		if !equalDeferred(result.Deferred, first.Deferred) {
			t.Fatalf("run %d: Deferred = %v, want %v", i, result.Deferred, first.Deferred)
		}
	}
}

func equalDeferred(a, b []autoseed.DeferredReference) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
