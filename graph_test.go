package autoseed_test

import (
	"errors"
	"testing"

	"github.com/danellalc/autoseed"
)

func TestNewDependencyGraph_UnknownReference(t *testing.T) {
	_, err := autoseed.NewDependencyGraph([]autoseed.Entity{
		{
			Name: "Order",
			References: []autoseed.Reference{
				{Name: "CustomerID", Target: "Customer"},
			},
		},
	})
	if !errors.Is(err, autoseed.ErrUnknownReference) {
		t.Fatalf("got %v, want ErrUnknownReference", err)
	}
}

func TestResolve_Order(t *testing.T) {
	tests := []struct {
		name     string
		entities []autoseed.Entity
		want     []string
	}{
		{
			name: "no references, ties break on name",
			entities: []autoseed.Entity{
				{Name: "Zebra"},
				{Name: "Apple"},
			},
			want: []string{"Apple", "Zebra"},
		},
		{
			name: "linear chain",
			entities: []autoseed.Entity{
				{Name: "OrderItem", References: []autoseed.Reference{{Name: "OrderID", Target: "Order"}}},
				{Name: "Order", References: []autoseed.Reference{{Name: "CustomerID", Target: "Customer"}}},
				{Name: "Customer"},
			},
			want: []string{"Customer", "Order", "OrderItem"},
		},
		{
			name: "diamond, ties break on name",
			entities: []autoseed.Entity{
				{Name: "Root"},
				{Name: "Left", References: []autoseed.Reference{{Name: "RootID", Target: "Root"}}},
				{Name: "Right", References: []autoseed.Reference{{Name: "RootID", Target: "Root"}}},
				{Name: "Bottom", References: []autoseed.Reference{
					{Name: "LeftID", Target: "Left"},
					{Name: "RightID", Target: "Right"},
				}},
			},
			want: []string{"Root", "Left", "Right", "Bottom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := autoseed.NewDependencyGraph(tt.entities)
			if err != nil {
				t.Fatalf("NewDependencyGraph: %v", err)
			}
			result, err := graph.Resolve()
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if !equalStrings(result.Order, tt.want) {
				t.Fatalf("Order = %v, want %v", result.Order, tt.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
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
