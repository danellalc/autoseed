// Package inference generates field values through gofakeit, seeded
// hierarchically from an autoseed.SeededSource, so autoseed's core stays
// testable without a value generator: the same separation the .NET sibling
// keeps between AutoSeed.Core and AutoSeed.Inference.
package inference

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/danellalc/autoseed"
)

// Rule infers a value for one field. CanInfer is a pure predicate over the
// field's name and type; once it claims a field for a row, no other rule
// gets a turn at it, so Infer must always return something usable.
type Rule interface {
	Priority() int
	CanInfer(field autoseed.Field) bool
	Infer(field autoseed.Field, seed *autoseed.SeededSource, generated map[string]any) any
}

// Generator produces a full row of values for an entity. Rules run in
// ascending Priority order, and within a tier in registration order; a
// rule completes across every field in the entity before the next rule
// runs, so a higher-priority rule can read a value a lower-priority rule
// already produced this row (FirstName before Email, CreatedAt before
// UpdatedAt), never the reverse.
type Generator struct {
	rules []Rule
}

// NewGenerator returns a Generator trying rules in ascending Priority
// order.
func NewGenerator(rules ...Rule) *Generator {
	sorted := append([]Rule(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority() < sorted[j].Priority() })
	return &Generator{rules: sorted}
}

// GenerateRow produces a value for every field on entity that isn't
// database-generated: a foreign key column (copied from its parent at
// the persistence stage, never from inference) or an auto-increment
// primary key. A primary key field that is neither — a natural,
// non-auto-increment column, whether it's an entity's whole key or, in a
// composite key, the one part not already covered by a reference — is
// generated like any other field, since leaving it at its Go zero value
// would make every row collide on it.
//
// A field typed as one of database/sql's nullable wrappers (sql.NullString,
// sql.NullInt64, sql.NullBool, sql.NullFloat64, sql.NullTime, and the
// narrower NullInt32/NullInt16/NullByte) is matched against rules by its
// wrapped value's type, so a field named Email of type sql.NullString
// still gets EmailRule's treatment, not just generic text, and always
// comes back Valid — there is no null-rate knob yet.
//
// seed must already be scoped to the row, typically
// source.Entity(entity.Name).Row(index). It returns
// autoseed.ErrUnsupportedField, naming the entity and field, if no rule
// claims a field. A string value longer than field.Size is truncated
// before it reaches the caller, regardless of which rule produced it, so a
// named rule can never hand a sized column a value the database rejects.
func (g *Generator) GenerateRow(entity autoseed.Entity, seed *autoseed.SeededSource) (map[string]any, error) {
	skip := foreignKeyFieldNames(entity)

	fields := make([]autoseed.Field, len(entity.Fields))
	nullableFields := make(map[string]reflect.Type, len(entity.Fields))
	for i, field := range entity.Fields {
		if isNullableWrapper(field.Type) {
			nullableFields[field.Name] = field.Type
			field.Type = unwrapNullable(field.Type)
		}
		fields[i] = field
	}

	values := make(map[string]any, len(fields))
	claimed := make(map[string]bool, len(fields))

	for _, rule := range g.rules {
		for _, field := range fields {
			if databaseGenerated(field) || skip[field.Name] || claimed[field.Name] || !rule.CanInfer(field) {
				continue
			}
			claimed[field.Name] = true
			values[field.Name] = truncate(field, rule.Infer(field, seed.Field(field.Name), values))
		}
	}

	for _, field := range fields {
		if databaseGenerated(field) || skip[field.Name] || claimed[field.Name] {
			continue
		}
		return nil, fmt.Errorf("%w: %s.%s", autoseed.ErrUnsupportedField, entity.Name, field.Name)
	}

	for name, nullableType := range nullableFields {
		if values[name] == nil {
			continue
		}
		values[name] = wrapNullable(nullableType, values[name])
	}

	return values, nil
}

func databaseGenerated(field autoseed.Field) bool {
	return field.PrimaryKey && field.AutoIncrement
}

func foreignKeyFieldNames(entity autoseed.Entity) map[string]bool {
	names := make(map[string]bool)
	for _, ref := range entity.References {
		for _, name := range ref.Fields {
			names[name] = true
		}
	}
	return names
}
