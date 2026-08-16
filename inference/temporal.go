package inference

import (
	"time"

	"github.com/danellalc/autoseed"
)

var referenceNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const (
	createdAtWindow = 2 * 365 * 24 * time.Hour
	updatedAtMaxLag = 90 * 24 * time.Hour
)

// CreatedAtRule infers a creation timestamp for a time.Time field ending
// in CreatedAt, within the two years before a fixed reference time.
type CreatedAtRule struct{}

// Priority runs CreatedAtRule before UpdatedAtRule, which reads its output
// back.
func (CreatedAtRule) Priority() int { return 0 }

// CanInfer matches a time.Time field ending in CreatedAt.
func (CreatedAtRule) CanInfer(field autoseed.Field) bool {
	return isTime(field.Type) && hasSuffix(field.Name, "CreatedAt")
}

// Infer generates a timestamp within the two years before a fixed
// reference time.
func (CreatedAtRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return randomPastTime(seed)
}

// UpdatedAtRule infers an update timestamp for a time.Time field ending in
// UpdatedAt: at or after a same-row CreatedAt when one was generated,
// otherwise drawn independently from the same window CreatedAtRule uses.
type UpdatedAtRule struct{}

// Priority runs UpdatedAtRule after CreatedAtRule, so a same-row CreatedAt
// is already generated when Infer looks for it.
func (UpdatedAtRule) Priority() int { return 1 }

// CanInfer matches a time.Time field ending in UpdatedAt.
func (UpdatedAtRule) CanInfer(field autoseed.Field) bool {
	return isTime(field.Type) && hasSuffix(field.Name, "UpdatedAt")
}

// Infer generates a timestamp at or after a same-row CreatedAt when one
// was already generated, otherwise independently within the same window
// CreatedAtRule uses.
func (UpdatedAtRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, generated map[string]any) any {
	createdAt, ok := findSibling[time.Time](generated, "CreatedAt")
	if !ok {
		return randomPastTime(seed)
	}
	lag := time.Duration(seed.Rand().Int64N(int64(updatedAtMaxLag)))
	return createdAt.Add(lag)
}

type genericTimeRule struct{}

func (genericTimeRule) Priority() int { return 10 }

func (genericTimeRule) CanInfer(field autoseed.Field) bool {
	return isTime(field.Type)
}

func (genericTimeRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return randomPastTime(seed)
}

func randomPastTime(seed *autoseed.SeededSource) time.Time {
	offset := time.Duration(seed.Rand().Int64N(int64(createdAtWindow)))
	return referenceNow.Add(-offset)
}
