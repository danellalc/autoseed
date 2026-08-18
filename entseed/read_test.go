package entseed

import (
	"testing"

	"github.com/danellalc/autoseed"
)

// TestRead_FieldNamesArePascalCasedForRuleMatching guards a real bug: a
// field an idiomatic ent schema declares snake_case (Customer's own
// "first_name", "last_name") used to reach autoseed.Field.Name verbatim,
// with the underscore intact -- but every named inference rule (NameRule,
// EmailRule, CreatedAtRule, ...) matches a suffix like "FirstName" or
// "CreatedAt" the way a GORM struct field, always already PascalCase,
// would spell it. "first_name" never matches "...firstname" as a
// continuous string, so every such rule silently never fired for a
// snake_case ent schema -- the common case, not an edge one -- falling
// back to the generic, uncorrelated rules instead. Fields are now
// pascal-cased the same way an edge's own Reference.Fields already is.
func TestRead_FieldNamesArePascalCasedForRuleMatching(t *testing.T) {
	entities, _, _, err := read("./internal/entfixtures/schema")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var customer autoseed.Entity
	found := false
	for _, e := range entities {
		if e.Name == "Customer" {
			customer = e
			found = true
		}
	}
	if !found {
		t.Fatal("no Customer entity in read() output")
	}

	for _, want := range []string{"FirstName", "LastName"} {
		hasField := false
		for _, f := range customer.Fields {
			if f.Name == want {
				hasField = true
			}
		}
		if !hasField {
			t.Fatalf("Customer has no field named %q (declared snake_case in the schema); fields: %v", want, customer.Fields)
		}
	}
}

// TestRead_CompositeUniqueConstraint guards that an index.Fields(...).Unique()
// index declared on an ent schema's Indexes method is read into
// Entity.UniqueConstraints, in the index's own column order, using each
// field's pascal-cased ent name -- Number declares an explicit
// StorageKey that differs from "number", so this also guards that
// resolution goes through StorageKey rather than assuming it equals the
// field's Name. Neither field alone is marked Field.Unique -- Invoice.
// Series and Invoice.Number are only unique together.
func TestRead_CompositeUniqueConstraint(t *testing.T) {
	entities, _, _, err := read("./internal/entfixtures/schema")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var invoice autoseed.Entity
	found := false
	names := make([]string, 0, len(entities))
	for _, e := range entities {
		names = append(names, e.Name)
		if e.Name == "Invoice" {
			invoice = e
			found = true
		}
	}
	if !found {
		t.Fatalf("no Invoice entity in %v", names)
	}

	if len(invoice.UniqueConstraints) != 1 {
		t.Fatalf("UniqueConstraints = %v, want exactly one composite constraint", invoice.UniqueConstraints)
	}
	if got := invoice.UniqueConstraints[0]; len(got) != 2 || got[0] != "Series" || got[1] != "Number" {
		t.Fatalf("UniqueConstraints[0] = %v, want [Series Number]", got)
	}

	for _, f := range invoice.Fields {
		if (f.Name == "Series" || f.Name == "Number") && f.Unique {
			t.Fatalf("%s must not be marked Field.Unique on its own -- it is only unique combined with the other", f.Name)
		}
	}
}

// TestRead_CompositeUniqueConstraintOnEdges guards that an
// index.Edges(...).Unique() index -- a composite unique constraint over
// edge-owned foreign key columns, not plain fields -- resolves each
// column to the SAME pascal-cased edge name buildEntity already gives
// that edge's own Reference.Fields, not the edge's underlying storage
// column name. If these two disagreed, junctionCap could never match the
// UniqueConstraints entry back to entity.References by Fields.
func TestRead_CompositeUniqueConstraintOnEdges(t *testing.T) {
	entities, _, _, err := read("./internal/entfixtures/schema")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var assignment autoseed.Entity
	found := false
	for _, e := range entities {
		if e.Name == "Assignment" {
			assignment = e
			found = true
		}
	}
	if !found {
		t.Fatal("no Assignment entity in read() output")
	}

	if len(assignment.UniqueConstraints) != 1 {
		t.Fatalf("UniqueConstraints = %v, want exactly one composite constraint", assignment.UniqueConstraints)
	}
	got := assignment.UniqueConstraints[0]
	if len(got) != 3 {
		t.Fatalf("UniqueConstraints[0] = %v, want 3 fields", got)
	}

	referenceFields := make(map[string]bool, len(assignment.References))
	for _, ref := range assignment.References {
		if len(ref.Fields) != 1 {
			t.Fatalf("Assignment reference %+v has more than one field, want exactly one (ent edges are always single-column)", ref)
		}
		referenceFields[ref.Fields[0]] = true
	}
	for _, field := range got {
		if !referenceFields[field] {
			t.Fatalf("UniqueConstraints[0] names %q, which does not match any Reference.Fields entry %v -- edge-index resolution disagrees with buildEntity's own Reference.Fields naming", field, assignment.References)
		}
	}
}
