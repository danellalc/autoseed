package gormseed_test

import (
	"context"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
	"gorm.io/gorm"
)

// CoverageCustomer exercises every field-level axis SeedCoverage
// promises: Bio is Nullable (a pointer), Active is bool-kind, Code is a
// sized string. CoverageOrder gives Customer a downstream dependent, so
// its own relationship-cardinality axis (zero/one/many children) is
// exercised too.
type CoverageCustomer struct {
	ID     uint `gorm:"primaryKey"`
	Bio    *string
	Active bool
	Code   string `gorm:"size:5"`
}

type CoverageOrder struct {
	ID         uint `gorm:"primaryKey"`
	CustomerID uint `gorm:"not null"`
	Customer   CoverageCustomer
}

// TestSeedCoverage_Postgres_ExercisesEveryAxis guards the end-to-end
// path: real rows, written through a real Postgres connection, actually
// carry the boundary values PlanCoverage/ApplyCoverageOverrides promise
// -- not just that the in-memory plan/row maps look right before
// persistence ever touches them.
func TestSeedCoverage_Postgres_ExercisesEveryAxis(t *testing.T) {
	db := postgresDB(t)
	models := []any{&CoverageCustomer{}, &CoverageOrder{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.SeedCoverage(context.Background(), db, models, autoseed.WithSeed(1)); err != nil {
		t.Fatalf("SeedCoverage: %v", err)
	}

	var customers []CoverageCustomer
	if err := db.Find(&customers).Error; err != nil {
		t.Fatalf("querying customers: %v", err)
	}
	if len(customers) < 3 {
		t.Fatalf("Customer count = %d, want at least 3 (drives Order's zero/one/many coverage)", len(customers))
	}
	if len(customers) > 10 {
		t.Fatalf("Customer count = %d, want a small coverage dataset, not a bulk one", len(customers))
	}

	sawNilBio, sawValueBio := false, false
	sawActive, sawInactive := false, false
	sawEmptyCode, sawOneCharCode, sawMaxCode := false, false, false
	for _, c := range customers {
		if c.Bio == nil {
			sawNilBio = true
		} else {
			sawValueBio = true
		}
		if c.Active {
			sawActive = true
		} else {
			sawInactive = true
		}
		switch len(c.Code) {
		case 0:
			sawEmptyCode = true
		case 1:
			sawOneCharCode = true
		case 5:
			sawMaxCode = true
		}
	}
	if !sawNilBio || !sawValueBio {
		t.Fatalf("Bio: want both nil and non-nil across customers, sawNil=%v sawValue=%v", sawNilBio, sawValueBio)
	}
	if !sawActive || !sawInactive {
		t.Fatalf("Active: want both true and false across customers, sawActive=%v sawInactive=%v", sawActive, sawInactive)
	}
	if !sawEmptyCode || !sawOneCharCode || !sawMaxCode {
		t.Fatalf("Code: want empty, one-character and 5-character (Size) values across customers, sawEmpty=%v sawOneChar=%v sawMax=%v", sawEmptyCode, sawOneCharCode, sawMaxCode)
	}

	var orders []CoverageOrder
	if err := db.Find(&orders).Error; err != nil {
		t.Fatalf("querying orders: %v", err)
	}
	ordersPerCustomer := make(map[uint]int, len(customers))
	for _, c := range customers {
		ordersPerCustomer[c.ID] = 0
	}
	for _, o := range orders {
		ordersPerCustomer[o.CustomerID]++
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
	db1 := postgresDB(t)
	db2 := postgresDB(t)
	models := []any{&CoverageCustomer{}, &CoverageOrder{}}

	for _, db := range []*gorm.DB{db1, db2} {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("AutoMigrate: %v", err)
		}
		if err := gormseed.SeedCoverage(context.Background(), db, models, autoseed.WithSeed(7)); err != nil {
			t.Fatalf("SeedCoverage: %v", err)
		}
	}

	var customers1, customers2 []CoverageCustomer
	db1.Order("id").Find(&customers1)
	db2.Order("id").Find(&customers2)
	if len(customers1) != len(customers2) {
		t.Fatalf("Customer count differs across runs with the same seed: %d vs %d", len(customers1), len(customers2))
	}
	for i := range customers1 {
		if customers1[i].Code != customers2[i].Code || customers1[i].Active != customers2[i].Active {
			t.Fatalf("row %d differs across runs: %+v vs %+v", i, customers1[i], customers2[i])
		}
	}
}
