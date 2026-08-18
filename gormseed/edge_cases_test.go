package gormseed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
)

type EdgeSolo struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

// TestSeed_NilDB guards a real bug: Seed used to panic deep inside GORM
// (a nil pointer dereference in db.Table) instead of returning a clean,
// named error. Unlike Explain, which only inspects Go struct types and
// genuinely works with a nil db, Seed writes rows and needs a connection.
func TestSeed_NilDB(t *testing.T) {
	err := gormseed.Seed(context.Background(), nil, []any{&EdgeSolo{}}, autoseed.WithScale(10))
	if !errors.Is(err, gormseed.ErrNilDB) {
		t.Fatalf("Seed(nil db) = %v, want ErrNilDB", err)
	}
}

func TestSeed_EmptyModels(t *testing.T) {
	db := postgresDB(t)
	if err := gormseed.Seed(context.Background(), db, []any{}, autoseed.WithScale(10)); err != nil {
		t.Fatalf("Seed(empty models) = %v, want a no-op", err)
	}
}

func TestExplain_EmptyModels(t *testing.T) {
	plan, err := gormseed.Explain(nil, []any{})
	if err != nil {
		t.Fatalf("Explain(empty models): %v", err)
	}
	if len(plan.Order) != 0 {
		t.Fatalf("Order = %v, want empty", plan.Order)
	}
}

func TestSeed_ScaleZero(t *testing.T) {
	db := postgresDB(t)
	models := []any{&EdgeSolo{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithScale(0)); err != nil {
		t.Fatalf("Seed(scale=0): %v", err)
	}
	var count int64
	db.Model(&EdgeSolo{}).Count(&count)
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

type EdgeAllNullableParent struct {
	ID uint `gorm:"primaryKey"`
}
type EdgeAllNullableChild struct {
	ID       uint `gorm:"primaryKey"`
	ParentID *uint
	Parent   *EdgeAllNullableParent
}

// TestSeed_OnlyNullableReference covers an entity whose one reference is
// nullable, so it never has a driver: it must get the root scale directly,
// not zero and not a long-tail draw off a principal it isn't required to
// have.
func TestSeed_OnlyNullableReference(t *testing.T) {
	db := postgresDB(t)
	models := []any{&EdgeAllNullableParent{}, &EdgeAllNullableChild{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithScale(20)); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	var childCount int64
	db.Model(&EdgeAllNullableChild{}).Count(&childCount)
	if childCount != 20 {
		t.Fatalf("EdgeAllNullableChild count = %d, want 20 (root scale, since its only reference is nullable and never drives)", childCount)
	}
}

type EdgeZeroFieldEntity struct {
	ID uint `gorm:"primaryKey"`
}

// TestSeed_EntityWithOnlyPK stresses the PK-only-table batch-insert
// workaround (batchSizeFor) well past the default batch size, on a table
// with no column besides its primary key.
func TestSeed_EntityWithOnlyPK(t *testing.T) {
	db := postgresDB(t)
	models := []any{&EdgeZeroFieldEntity{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithScale(300)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var distinctIDs int64
	db.Raw(`SELECT count(DISTINCT id) FROM edge_zero_field_entities`).Scan(&distinctIDs)
	if distinctIDs != 300 {
		t.Fatalf("distinct IDs = %d, want 300 — batch readback must have populated every row's ID, not just the first (the PK-only-table gotcha)", distinctIDs)
	}
}

// TestSeed_ContextAlreadyCancelled guards a real bug: ctx was accepted by
// Seed but never threaded into the actual database calls (CreateInBatches,
// Transaction), so a caller's cancellation or deadline had no effect on
// in-flight writes.
func TestSeed_ContextAlreadyCancelled(t *testing.T) {
	db := postgresDB(t)
	models := []any{&EdgeSolo{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := gormseed.Seed(ctx, db, models, autoseed.WithScale(10)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Seed(cancelled ctx) = %v, want context.Canceled", err)
	}
}
