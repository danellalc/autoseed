package inference_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

type fixedRule struct {
	priority int
	claims   string
	value    any
}

func (r fixedRule) Priority() int { return r.priority }

func (r fixedRule) CanInfer(field autoseed.Field) bool {
	return field.Name == r.claims
}

func (r fixedRule) Infer(autoseed.Field, *autoseed.SeededSource, map[string]any) any {
	return r.value
}

func TestGenerateRow_LowerPriorityRunsFirstAcrossAllFields(t *testing.T) {
	var order []string
	first := recordingRule{priority: 0, claims: "A", order: &order}
	second := recordingRule{priority: 1, claims: "B", order: &order}

	gen := inference.NewGenerator(second, first)
	entity := autoseed.Entity{
		Name: "Widget",
		Fields: []autoseed.Field{
			{Name: "A", Type: reflect.TypeOf("")},
			{Name: "B", Type: reflect.TypeOf("")},
		},
	}

	if _, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if !equalStrings(order, []string{"A", "B"}) {
		t.Fatalf("claim order = %v, want [A B] (priority 0 before priority 1, regardless of registration order)", order)
	}
}

type recordingRule struct {
	priority int
	claims   string
	order    *[]string
}

func (r recordingRule) Priority() int { return r.priority }

func (r recordingRule) CanInfer(field autoseed.Field) bool { return field.Name == r.claims }

func (r recordingRule) Infer(field autoseed.Field, _ *autoseed.SeededSource, _ map[string]any) any {
	*r.order = append(*r.order, field.Name)
	return "value"
}

func TestGenerateRow_FirstClaimWins(t *testing.T) {
	first := fixedRule{priority: 0, claims: "Name", value: "first"}
	second := fixedRule{priority: 1, claims: "Name", value: "second"}

	gen := inference.NewGenerator(first, second)
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Name", Type: reflect.TypeOf("")}},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if values["Name"] != "first" {
		t.Fatalf("Name = %v, want %q (lower-priority rule claims first)", values["Name"], "first")
	}
}

// TestGenerateRow_TruncatesEveryRuleToFieldSize guards a real gap: only
// the generic fallback rule used to respect Field.Size, so a named rule
// (EmailRule, AddressRule, URLRule, ...) claiming a sized column could
// hand the database a value too long for it, turning a clean library-side
// concern into a raw driver constraint violation. Truncation now happens
// once, centrally in GenerateRow, so it applies no matter which rule
// produced the value — proven here with a rule GenerateRow has no special
// knowledge of.
func TestGenerateRow_TruncatesEveryRuleToFieldSize(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "Code", value: "abcdefghij"})
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Code", Type: reflect.TypeOf(""), Size: 5}},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if values["Code"] != "abcde" {
		t.Fatalf("Code = %q, want truncated to 5 chars (\"abcde\")", values["Code"])
	}
}

func TestGenerateRow_SkipsPrimaryKeyAndForeignKeyFields(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "Name", value: "x"})
	entity := autoseed.Entity{
		Name: "Order",
		Fields: []autoseed.Field{
			{Name: "ID", Type: reflect.TypeOf(0), PrimaryKey: true},
			{Name: "CustomerID", Type: reflect.TypeOf(0)},
			{Name: "Name", Type: reflect.TypeOf("")},
		},
		References: []autoseed.Reference{{Fields: []string{"CustomerID"}, Target: "Customer"}},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if _, ok := values["ID"]; ok {
		t.Fatal("ID is a primary key, must not be generated")
	}
	if _, ok := values["CustomerID"]; ok {
		t.Fatal("CustomerID is a foreign key, must not be generated")
	}
	if values["Name"] != "x" {
		t.Fatalf("Name = %v, want x", values["Name"])
	}
}

func TestGenerateRow_UnsupportedFieldFailsNamed(t *testing.T) {
	gen := inference.NewGenerator()
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Mystery", Type: reflect.TypeOf(complex128(0))}},
	}

	_, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if !errors.Is(err, autoseed.ErrUnsupportedField) {
		t.Fatalf("got %v, want ErrUnsupportedField", err)
	}
	if got := err.Error(); got == "" {
		t.Fatal("error must name the entity and field")
	}
}

// TestGenerateRow_NilFieldTypeFailsNamedNotPanics guards CanInfer's nil
// guard (match.go's isKind): a ModelSource that leaves Field.Type unset —
// legitimate, since the inference package is meant to be usable without
// gormseed — must fail as ErrUnsupportedField, never panic.
func TestGenerateRow_NilFieldTypeFailsNamedNotPanics(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Mystery"}},
	}

	_, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(1))
	if !errors.Is(err, autoseed.ErrUnsupportedField) {
		t.Fatalf("got %v, want ErrUnsupportedField", err)
	}
}

func TestDefaultGenerator_DifferentRowsAndEntitiesDiverge(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Name", Type: reflect.TypeOf("")}},
	}
	root := autoseed.NewSeededSource(42)
	gen := inference.NewDefaultGenerator()

	row0, err := gen.GenerateRow(entity, root.Entity(entity.Name).Row(0))
	if err != nil {
		t.Fatalf("GenerateRow row 0: %v", err)
	}
	row1, err := gen.GenerateRow(entity, root.Entity(entity.Name).Row(1))
	if err != nil {
		t.Fatalf("GenerateRow row 1: %v", err)
	}
	if row0["Name"] == row1["Name"] {
		t.Fatalf("row 0 and row 1 of %s both produced %v, want different values", entity.Name, row0["Name"])
	}

	other, err := gen.GenerateRow(autoseed.Entity{Name: "Gadget", Fields: entity.Fields}, root.Entity("Gadget").Row(0))
	if err != nil {
		t.Fatalf("GenerateRow other entity: %v", err)
	}
	if row0["Name"] == other["Name"] {
		t.Fatalf("Widget row 0 and Gadget row 0 both produced %v, want different values", row0["Name"])
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
