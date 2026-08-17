package inference

import "github.com/danellalc/autoseed"

const softDeleteSurvivalRate = 0.9

// SoftDeleteRule infers a value for the field an adapter flagged as an
// entity's soft-delete column (Field.SoftDelete): nil for the large
// majority of rows, so a default query filter still finds them, and a
// past timestamp for a small minority, so the deleted branch of the
// model has real rows to test against. Same 90/10 split the .NET
// sibling defaults its query-filter pass rate to.
type SoftDeleteRule struct{}

// Priority runs SoftDeleteRule at the default tier: nothing produces a
// value it depends on, and nothing reads its own output back.
func (SoftDeleteRule) Priority() int { return 0 }

// CanInfer matches any field the adapter flagged as the soft-delete
// column, regardless of its name or Go type.
func (SoftDeleteRule) CanInfer(field autoseed.Field) bool {
	return field.SoftDelete
}

// Infer returns nil for most rows and a past timestamp for the rest.
// The field's own Set method (schema.Field.Set, backed by sql.Scanner
// for GORM's gorm.DeletedAt) turns nil into SQL NULL and a time.Time
// into a valid deleted-at timestamp.
func (SoftDeleteRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	if seed.Rand().Float64() < softDeleteSurvivalRate {
		return nil
	}
	return randomPastTime(seed)
}
