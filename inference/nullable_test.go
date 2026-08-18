package inference_test

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

// TestGenerateRow_NullableStringGetsNamedRuleTreatment guards a real gap:
// GenerateRow used to fail every field typed as one of database/sql's
// nullable wrappers (sql.NullString, sql.NullInt64, ...) with
// ErrUnsupportedField, since no rule's CanInfer matches a struct kind —
// blocking an extremely common, idiomatic Go/GORM nullable-column model
// outright. A field named Email of type sql.NullString must still get
// EmailRule's treatment (agreeing with a same-row FirstName/LastName),
// not just fail or fall to generic text.
func TestGenerateRow_NullableStringGetsNamedRuleTreatment(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Customer",
		Fields: []autoseed.Field{
			{Name: "FirstName", Type: reflect.TypeOf("")},
			{Name: "LastName", Type: reflect.TypeOf("")},
			{Name: "Email", Type: reflect.TypeOf(sql.NullString{})},
		},
	}

	values, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(1).Entity("Customer").Row(0))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}

	email, ok := values["Email"].(sql.NullString)
	if !ok {
		t.Fatalf("Email = %#v (%T), want sql.NullString", values["Email"], values["Email"])
	}
	if !email.Valid {
		t.Fatal("Email.Valid = false, want true — no null-rate knob exists yet, every field always gets a value")
	}

	first := strings.ToLower(values["FirstName"].(string))
	last := strings.ToLower(values["LastName"].(string))
	wantPrefix := first + "." + last + "@"
	if !strings.HasPrefix(email.String, wantPrefix) {
		t.Fatalf("Email.String = %q, want it to start with %q (EmailRule agreeing with the same-row FirstName/LastName)", email.String, wantPrefix)
	}
}

func TestGenerateRow_NullableNumericAndTemporalTypesWrapCorrectly(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Widget",
		Fields: []autoseed.Field{
			{Name: "Count", Type: reflect.TypeOf(sql.NullInt64{})},
			{Name: "Active", Type: reflect.TypeOf(sql.NullBool{})},
			{Name: "Weight", Type: reflect.TypeOf(sql.NullFloat64{})},
			{Name: "ShippedAt", Type: reflect.TypeOf(sql.NullTime{})},
		},
	}

	values, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(1).Entity("Widget").Row(0))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}

	count, ok := values["Count"].(sql.NullInt64)
	if !ok || !count.Valid {
		t.Fatalf("Count = %#v, want a Valid sql.NullInt64", values["Count"])
	}
	active, ok := values["Active"].(sql.NullBool)
	if !ok || !active.Valid {
		t.Fatalf("Active = %#v, want a Valid sql.NullBool", values["Active"])
	}
	weight, ok := values["Weight"].(sql.NullFloat64)
	if !ok || !weight.Valid {
		t.Fatalf("Weight = %#v, want a Valid sql.NullFloat64", values["Weight"])
	}
	shippedAt, ok := values["ShippedAt"].(sql.NullTime)
	if !ok || !shippedAt.Valid || shippedAt.Time.IsZero() {
		t.Fatalf("ShippedAt = %#v, want a Valid, non-zero sql.NullTime", values["ShippedAt"])
	}
}

// TestGenerateRow_NullableStringRespectsSize guards that truncation still
// applies to the wrapped value's underlying string, not just a plain
// string field.
func TestGenerateRow_NullableStringRespectsSize(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "Code", value: "abcdefghij"})
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Code", Type: reflect.TypeOf(sql.NullString{}), Size: 5}},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	code, ok := values["Code"].(sql.NullString)
	if !ok {
		t.Fatalf("Code = %#v, want sql.NullString", values["Code"])
	}
	if code.String != "abcde" {
		t.Fatalf("Code.String = %q, want truncated to 5 chars (\"abcde\")", code.String)
	}
}

func TestGenerateRow_NullableWrapperDeterministic(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "ShippedAt", Type: reflect.TypeOf(sql.NullTime{})}},
	}

	first, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(7).Entity("Widget").Row(0))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(7).Entity("Widget").Row(0))
		if err != nil {
			t.Fatalf("run %d: GenerateRow: %v", i, err)
		}
		if !got["ShippedAt"].(sql.NullTime).Time.Equal(first["ShippedAt"].(sql.NullTime).Time) {
			t.Fatalf("run %d: ShippedAt = %v, want %v", i, got["ShippedAt"], first["ShippedAt"])
		}
	}
}
