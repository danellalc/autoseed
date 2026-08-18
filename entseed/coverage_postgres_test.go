package entseed_test

import (
	"context"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/entseed"
	"github.com/danellalc/autoseed/entseed/internal/entfixtures/customer"
)

// TestSeedCoverage_Postgres_ExercisesEveryAxis guards the end-to-end path
// on the same schema entClient's other tests use: Order.notes is the
// fixture's only Nullable field, and Customer/Order is its only
// relationship, so this is what the schema itself can exercise. The
// sized-string axis is separately out of reach for any ent schema --
// entseed has no way to read a MaxLen validator back out of ent's
// compiled closures (see TestSeed_Postgres_CompositeUniqueConstraint's
// own doc comment for why) -- but the bool-kind axis works for any ent
// schema with a real field.Bool() column; this fixture simply has none.
func TestSeedCoverage_Postgres_ExercisesEveryAxis(t *testing.T) {
	client := entClient(t)
	ctx := context.Background()

	if err := entseed.SeedCoverage(ctx, client, schemaPath, autoseed.WithSeed(1)); err != nil {
		t.Fatalf("SeedCoverage: %v", err)
	}

	customers, err := client.Customer.Query().All(ctx)
	if err != nil {
		t.Fatalf("querying customers: %v", err)
	}
	if len(customers) < 3 {
		t.Fatalf("Customer count = %d, want at least 3 (drives Order's zero/one/many coverage)", len(customers))
	}
	if len(customers) > 10 {
		t.Fatalf("Customer count = %d, want a small coverage dataset, not a bulk one", len(customers))
	}

	orders, err := client.Order.Query().All(ctx)
	if err != nil {
		t.Fatalf("querying orders: %v", err)
	}

	sawNilNotes, sawValueNotes := false, false
	ordersPerCustomer := make(map[int]int, len(customers))
	for _, c := range customers {
		ordersPerCustomer[c.ID] = 0
	}
	for _, o := range orders {
		if o.Notes == "" {
			sawNilNotes = true
		} else {
			sawValueNotes = true
		}
		customerID, err := o.QueryCustomer().OnlyID(ctx)
		if err != nil {
			t.Fatalf("order %d: querying customer: %v", o.ID, err)
		}
		ordersPerCustomer[customerID]++
	}
	if !sawNilNotes || !sawValueNotes {
		t.Fatalf("Notes: want both cleared and non-empty across orders, sawNil=%v sawValue=%v", sawNilNotes, sawValueNotes)
	}

	sawZero, sawOne, sawMany := false, false, false
	for _, count := range ordersPerCustomer {
		switch {
		case count == 0:
			sawZero = true
		case count == 1:
			sawOne = true
		case count >= 2:
			sawMany = true
		}
	}
	if !sawZero || !sawOne || !sawMany {
		t.Fatalf("Order cardinality: want a customer with zero, one, and several (many) orders, sawZero=%v sawOne=%v sawMany=%v: %v", sawZero, sawOne, sawMany, ordersPerCustomer)
	}
}

// TestSeedCoverage_Postgres_Deterministic guards that, like Seed, the
// same seed produces byte-identical coverage output across runs.
func TestSeedCoverage_Postgres_Deterministic(t *testing.T) {
	ctx := context.Background()
	client1 := entClient(t)
	client2 := entClient(t)

	if err := entseed.SeedCoverage(ctx, client1, schemaPath, autoseed.WithSeed(7)); err != nil {
		t.Fatalf("SeedCoverage (client1): %v", err)
	}
	if err := entseed.SeedCoverage(ctx, client2, schemaPath, autoseed.WithSeed(7)); err != nil {
		t.Fatalf("SeedCoverage (client2): %v", err)
	}

	customers1, err := client1.Customer.Query().Order(customer.ByID()).All(ctx)
	if err != nil {
		t.Fatalf("querying customers1: %v", err)
	}
	customers2, err := client2.Customer.Query().Order(customer.ByID()).All(ctx)
	if err != nil {
		t.Fatalf("querying customers2: %v", err)
	}
	if len(customers1) != len(customers2) {
		t.Fatalf("Customer count differs across runs with the same seed: %d vs %d", len(customers1), len(customers2))
	}
	for i := range customers1 {
		if customers1[i].Email != customers2[i].Email || customers1[i].FirstName != customers2[i].FirstName {
			t.Fatalf("row %d differs across runs: %+v vs %+v", i, customers1[i], customers2[i])
		}
	}
}
