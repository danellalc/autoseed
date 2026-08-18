package gormseed

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func parseForTest(t *testing.T, model any) *schema.Schema {
	t.Helper()
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse: %v", err)
	}
	return s
}

type batchSizeAutoIncrementOnly struct {
	ID uint `gorm:"primaryKey"`
}

type batchSizeCompositeKeyNoOtherColumn struct {
	LeftID  uint `gorm:"primaryKey"`
	RightID uint `gorm:"primaryKey"`
}

type batchSizeCompositeKeyWithOtherColumn struct {
	LeftID  uint `gorm:"primaryKey"`
	RightID uint `gorm:"primaryKey"`
	Note    string
}

// TestBatchSizeFor_OnlyAutoIncrementColumnsForceSingleRow guards the real
// GORM/Postgres limitation ARCHITECTURE.md documents: a table with no
// column besides an auto-increment primary key makes GORM emit
// INSERT ... DEFAULT VALUES, which has no multi-row form and only reports
// the first row's generated ID back.
func TestBatchSizeFor_OnlyAutoIncrementColumnsForceSingleRow(t *testing.T) {
	s := parseForTest(t, &batchSizeAutoIncrementOnly{})
	if got := batchSizeFor(s); got != 1 {
		t.Fatalf("batchSizeFor = %d, want 1", got)
	}
}

// TestBatchSizeFor_NonAutoIncrementCompositeKeyBatchesNormally guards a
// real bug: batchSizeFor used to check PrimaryKey instead of
// AutoIncrement, so any composite-primary-key entity with no other
// column — a many-to-many join table, an "attributed join" entity with
// no extra data, a shared-primary-key one-to-one — fell into the
// DEFAULT-VALUES-only branch and inserted one row at a time, even though
// every one of its columns has a real, non-omitted value and batches
// exactly as well as any other table.
func TestBatchSizeFor_NonAutoIncrementCompositeKeyBatchesNormally(t *testing.T) {
	s := parseForTest(t, &batchSizeCompositeKeyNoOtherColumn{})
	if got := batchSizeFor(s); got != insertBatchSize {
		t.Fatalf("batchSizeFor = %d, want %d (both key columns have real values, no DEFAULT VALUES risk)", got, insertBatchSize)
	}
}

func TestBatchSizeFor_CompositeKeyWithDataColumnBatchesNormally(t *testing.T) {
	s := parseForTest(t, &batchSizeCompositeKeyWithOtherColumn{})
	if got := batchSizeFor(s); got != insertBatchSize {
		t.Fatalf("batchSizeFor = %d, want %d", got, insertBatchSize)
	}
}
