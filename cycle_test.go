package autoseed_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
)

func TestResolve_NullableSelfReference(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Employee",
			References: []autoseed.Reference{
				{Fields: []string{"ManagerID"}, Target: "Employee", Nullable: true},
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
	want := []autoseed.DeferredReference{{Entity: "Employee", Fields: []string{"ManagerID"}, Target: "Employee"}}
	if !equalDeferred(result.Deferred, want) {
		t.Fatalf("Deferred = %v, want %v", result.Deferred, want)
	}
}

func TestResolve_RequiredSelfReference(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Employee",
			References: []autoseed.Reference{
				{Fields: []string{"ManagerID"}, Target: "Employee", Nullable: false},
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
				{Fields: []string{"ContactID"}, Target: "Contact", Nullable: false},
			},
		},
		{
			Name: "Contact",
			References: []autoseed.Reference{
				{Fields: []string{"DefaultOrderID"}, Target: "Order", Nullable: true},
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
	want := []autoseed.DeferredReference{{Entity: "Contact", Fields: []string{"DefaultOrderID"}, Target: "Order"}}
	if !equalDeferred(result.Deferred, want) {
		t.Fatalf("Deferred = %v, want %v", result.Deferred, want)
	}
}

func TestResolve_RequiredCrossEntityCycle(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Order",
			References: []autoseed.Reference{
				{Fields: []string{"ContactID"}, Target: "Contact", Nullable: false},
			},
		},
		{
			Name: "Contact",
			References: []autoseed.Reference{
				{Fields: []string{"DefaultOrderID"}, Target: "Order", Nullable: false},
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

// TestResolve_CycleErrorNamesRealPath guards against naming the cycle by
// alphabetically sorting its members: A, B, C sorted alphabetically already
// reads A->B->C, which would hide a real A->C->B->A cycle behind a
// plausible but false chain. The three entities here are deliberately
// wired so the real cycle (A->C->B->A) differs from alphabetical order.
func TestResolve_CycleErrorNamesRealPath(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{Name: "A", References: []autoseed.Reference{{Fields: []string{"CID"}, Target: "C"}}},
		{Name: "C", References: []autoseed.Reference{{Fields: []string{"BID"}, Target: "B"}}},
		{Name: "B", References: []autoseed.Reference{{Fields: []string{"AID"}, Target: "A"}}},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	_, err = graph.Resolve()
	if !errors.Is(err, autoseed.ErrUnsatisfiableCycle) {
		t.Fatalf("got %v, want ErrUnsatisfiableCycle", err)
	}
	want := "A -> C -> B -> A"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain the real cycle path %q", err.Error(), want)
	}
}

// TestResolve_MultiNullableEdgeCycleDefersOnlyOne guards against
// over-deferring: a 3-entity ring with two nullable edges (X->Y, Y->Z) and
// one required edge (Z->X) needs only one of the two nullable edges broken
// to become acyclic. Deferring both — the old behavior — would strip Y of
// its only reference, turning it into a driverless root entity with a flat
// row count instead of one derived from Z's. The .NET sibling's
// CycleResolver breaks exactly one edge per cycle, ordered by (dependent,
// principal, FK fields); X.YID sorts first, so it is the one deferred.
func TestResolve_MultiNullableEdgeCycleDefersOnlyOne(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{Name: "X", References: []autoseed.Reference{{Fields: []string{"YID"}, Target: "Y", Nullable: true}}},
		{Name: "Y", References: []autoseed.Reference{{Fields: []string{"ZID"}, Target: "Z", Nullable: true}}},
		{Name: "Z", References: []autoseed.Reference{{Fields: []string{"XID"}, Target: "X", Nullable: false}}},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	result, err := graph.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := []autoseed.DeferredReference{{Entity: "X", Fields: []string{"YID"}, Target: "Y"}}
	if !equalDeferred(result.Deferred, want) {
		t.Fatalf("Deferred = %v, want exactly %v (only X.YID broken, Y.ZID left intact)", result.Deferred, want)
	}
}

// TestResolve_ReportsEveryIndependentUnsatisfiableCycle guards against
// stopping at the first unsatisfiable cycle found: two disjoint required-FK
// pairs in the same model must both be named in one error, so fixing the
// first doesn't just reveal the second on the next Resolve call.
func TestResolve_ReportsEveryIndependentUnsatisfiableCycle(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{Name: "A", References: []autoseed.Reference{{Fields: []string{"BID"}, Target: "B", Nullable: false}}},
		{Name: "B", References: []autoseed.Reference{{Fields: []string{"AID"}, Target: "A", Nullable: false}}},
		{Name: "C", References: []autoseed.Reference{{Fields: []string{"DID"}, Target: "D", Nullable: false}}},
		{Name: "D", References: []autoseed.Reference{{Fields: []string{"CID"}, Target: "C", Nullable: false}}},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	_, err = graph.Resolve()
	if !errors.Is(err, autoseed.ErrUnsatisfiableCycle) {
		t.Fatalf("got %v, want ErrUnsatisfiableCycle", err)
	}
	for _, want := range []string{"A -> B -> A", "C -> D -> C"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err.Error(), want)
		}
	}
}

func TestResolve_DeduplicatesIdenticalDeferredReferences(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "A",
			References: []autoseed.Reference{
				{Fields: []string{"BID"}, Target: "B", Nullable: true},
				{Fields: []string{"BID"}, Target: "B", Nullable: true},
			},
		},
		{
			Name: "B",
			References: []autoseed.Reference{
				{Fields: []string{"AID"}, Target: "A", Nullable: false},
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
	if len(result.Deferred) != 1 {
		t.Fatalf("Deferred = %v, want exactly one entry for the duplicated reference", result.Deferred)
	}
}

func TestResolve_CompositeForeignKey(t *testing.T) {
	graph, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "OrderLine",
			References: []autoseed.Reference{
				{Fields: []string{"OrderID", "LineNo"}, Target: "Order", Nullable: false},
			},
		},
		{Name: "Order"},
	})
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}

	result, err := graph.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !equalStrings(result.Order, []string{"Order", "OrderLine"}) {
		t.Fatalf("Order = %v, want [Order OrderLine]", result.Order)
	}
	if len(result.Deferred) != 0 {
		t.Fatalf("Deferred = %v, want none", result.Deferred)
	}
}

// TestResolve_StableAcrossRuns guards the #1 determinism trap in this
// codebase: a map iterated somewhere in the resolution path would make this
// flaky. It is not.
func TestResolve_StableAcrossRuns(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Zebra", References: []autoseed.Reference{{Fields: []string{"RootID"}, Target: "Root"}}},
		{Name: "Mango", References: []autoseed.Reference{{Fields: []string{"RootID"}, Target: "Root"}}},
		{Name: "Apple", References: []autoseed.Reference{{Fields: []string{"RootID"}, Target: "Root"}}},
		{Name: "Root"},
		{
			Name: "Loop",
			References: []autoseed.Reference{
				{Fields: []string{"PeerID"}, Target: "Peer", Nullable: true},
			},
		},
		{
			Name: "Peer",
			References: []autoseed.Reference{
				{Fields: []string{"LoopID"}, Target: "Loop", Nullable: true},
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
		if a[i].Entity != b[i].Entity || a[i].Target != b[i].Target || !equalStrings(a[i].Fields, b[i].Fields) {
			return false
		}
	}
	return true
}
