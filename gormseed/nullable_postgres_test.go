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

// TestSeed_Postgres_NilRateNullsWrapperTypes guards the other end of null
// rate: at rate 1.0, every one of NullableProduct's database/sql wrapper
// fields comes back not Valid -- a real SQL NULL Postgres itself stored,
// not just an in-memory zero value.
func TestSeed_Postgres_NilRateNullsWrapperTypes(t *testing.T) {
	db := postgresDB(t)
	models := []any{&NullableProduct{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(1), autoseed.WithScale(20), autoseed.WithNilRate(1)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var products []NullableProduct
	db.Find(&products)
	if len(products) != 20 {
		t.Fatalf("count = %d, want 20", len(products))
	}
	for _, p := range products {
		if p.Email.Valid {
			t.Fatalf("product %d: Email = %#v, want not Valid at nil rate 1.0", p.ID, p.Email)
		}
		if p.Stock.Valid {
			t.Fatalf("product %d: Stock = %#v, want not Valid at nil rate 1.0", p.ID, p.Stock)
		}
		if p.InStock.Valid {
			t.Fatalf("product %d: InStock = %#v, want not Valid at nil rate 1.0", p.ID, p.InStock)
		}
		if p.Weight.Valid {
			t.Fatalf("product %d: Weight = %#v, want not Valid at nil rate 1.0", p.ID, p.Weight)
		}
		if p.RestockedAt.Valid {
			t.Fatalf("product %d: RestockedAt = %#v, want not Valid at nil rate 1.0", p.ID, p.RestockedAt)
		}
	}
}

// NullablePerson exercises a pointer-typed nullable field alongside a
// plain, non-pointer nullable-at-the-DB-level one, in one real model.
type NullablePerson struct {
	ID       uint `gorm:"primaryKey"`
	Nickname *string
	City     string
}

// TestSeed_Postgres_NilRateNullsPointerFieldButNotPlainField guards the
// pointer side of Field.Nullable's stricter definition, end to end: at
// nil rate 1.0, the pointer field Nickname is stored as a real SQL NULL,
// while City -- a plain string GORM also considers schema-nullable, but
// whose Go type has no way to represent absence -- still gets a real
// generated value every row, never silently degraded to an empty string
// that only looks like it was left out.
func TestSeed_Postgres_NilRateNullsPointerFieldButNotPlainField(t *testing.T) {
	db := postgresDB(t)
	models := []any{&NullablePerson{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(1), autoseed.WithScale(30), autoseed.WithNilRate(1)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var people []NullablePerson
	db.Find(&people)
	if len(people) != 30 {
		t.Fatalf("count = %d, want 30", len(people))
	}
	for _, p := range people {
		if p.Nickname != nil {
			t.Fatalf("person %d: Nickname = %v, want nil at nil rate 1.0", p.ID, *p.Nickname)
		}
		if p.City == "" {
			t.Fatalf("person %d: City is empty, want a real generated value -- a plain string field must never be silently left out", p.ID)
		}
	}
}
