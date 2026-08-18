package gormseed_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
)

// NullableProduct exercises every database/sql nullable wrapper type in
// one real GORM model — an extremely common, idiomatic way to represent
// a nullable column that GenerateRow used to reject outright with
// ErrUnsupportedField.
type NullableProduct struct {
	ID          uint           `gorm:"primaryKey"`
	Email       sql.NullString `gorm:"size:100"`
	Stock       sql.NullInt64
	InStock     sql.NullBool
	Weight      sql.NullFloat64
	RestockedAt sql.NullTime
}

func TestSeed_Postgres_NullableWrapperTypes(t *testing.T) {
	db := postgresDB(t)
	models := []any{&NullableProduct{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(1), autoseed.WithScale(20)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var products []NullableProduct
	db.Find(&products)
	if len(products) != 20 {
		t.Fatalf("count = %d, want 20", len(products))
	}
	for _, p := range products {
		if !p.Email.Valid || p.Email.String == "" {
			t.Fatalf("product %d: Email = %#v, want a Valid, non-empty value", p.ID, p.Email)
		}
		if !p.Stock.Valid {
			t.Fatalf("product %d: Stock = %#v, want Valid", p.ID, p.Stock)
		}
		if !p.InStock.Valid {
			t.Fatalf("product %d: InStock = %#v, want Valid", p.ID, p.InStock)
		}
		if !p.Weight.Valid {
			t.Fatalf("product %d: Weight = %#v, want Valid", p.ID, p.Weight)
		}
		if !p.RestockedAt.Valid || p.RestockedAt.Time.IsZero() {
			t.Fatalf("product %d: RestockedAt = %#v, want Valid and non-zero", p.ID, p.RestockedAt)
		}
	}
}
