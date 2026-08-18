package inference

import (
	"time"

	"github.com/danellalc/autoseed"
)

const softDeleteSurvivalRate = 0.9

// SoftDeleteRule infers a value for the field an adapter flagged as an
// entity's soft-delete column (Field.SoftDelete): nil for the large
// majority of rows, so a default query filter still finds them, and a
// timestamp for a small minority, so the deleted branch of the model has
// real rows to test against. Same 90/10 split the .NET sibling defaults
// its query-filter pass rate to.
type SoftDeleteRule struct{}

// Priority runs SoftDeleteRule after CreatedAtRule and UpdatedAtRule, so a
// same-row CreatedAt or UpdatedAt is already generated when Infer looks
// for it: a row cannot be deleted before it was last touched.
func (SoftDeleteRule) Priority() int { return 2 }

// CanInfer matches any field the adapter flagged as the soft-delete
// column, regardless of its name or Go type.
func (SoftDeleteRule) CanInfer(field autoseed.Field) bool {
	return field.SoftDelete
}

// Infer returns nil for most rows. For the rest, it returns a timestamp
// at or after a same-row UpdatedAt, or CreatedAt if there is no UpdatedAt,
// otherwise an independent past timestamp. The field's own Set method
// (schema.Field.Set, backed by sql.Scanner for GORM's gorm.DeletedAt)
// turns nil into SQL NULL and a time.Time into a valid deleted-at
// timestamp.
func (SoftDeleteRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, generated map[string]any) any {
	if seed.Rand().Float64() < softDeleteSurvivalRate {
		return nil
	}

	base, ok := findSibling[time.Time](generated, "UpdatedAt")
	if !ok {
		base, ok = findSibling[time.Time](generated, "CreatedAt")
	}
	if !ok {
		return randomPastTime(seed)
	}

	window := referenceNow.Sub(base)
	if window <= 0 {
		return base
	}
	return base.Add(time.Duration(seed.Rand().Int64N(int64(window))))
}
