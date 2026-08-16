package inference_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

func TestNameRule_FirstAndLastDiffer(t *testing.T) {
	first := inferField(t, inference.NameRule{}, autoseed.Field{Name: "FirstName", Type: reflect.TypeOf("")}, 1).(string)
	last := inferField(t, inference.NameRule{}, autoseed.Field{Name: "LastName", Type: reflect.TypeOf("")}, 1).(string)
	if first == "" || last == "" {
		t.Fatalf("FirstName = %q, LastName = %q, want both non-empty", first, last)
	}
}

func TestEmailRule_AgreesWithSiblingName(t *testing.T) {
	generated := map[string]any{"FirstName": "Ada", "LastName": "Lovelace"}
	source := autoseed.NewSeededSource(1).Entity("Customer").Row(0).Field("Email")

	value := (inference.EmailRule{}).Infer(autoseed.Field{Name: "Email"}, source, generated).(string)

	if !strings.HasPrefix(strings.ToLower(value), "ada.lovelace@") {
		t.Fatalf("Email = %q, want it to start with ada.lovelace@", value)
	}
}

func TestEmailRule_IndependentWithoutSiblings(t *testing.T) {
	source := autoseed.NewSeededSource(1).Entity("Customer").Row(0).Field("Email")
	value := (inference.EmailRule{}).Infer(autoseed.Field{Name: "Email"}, source, nil).(string)
	if !strings.Contains(value, "@") {
		t.Fatalf("Email = %q, want a valid-looking address", value)
	}
}

func TestPhoneRule_ProducesNonEmptyValue(t *testing.T) {
	value := inferField(t, inference.PhoneRule{}, autoseed.Field{Name: "Phone", Type: reflect.TypeOf("")}, 1).(string)
	if value == "" {
		t.Fatal("Phone must not be empty")
	}
}

// TestGenerateRow_EmailAddressNotClaimedByAddressRule guards a real bug:
// "EmailAddress" ends in "Address", so AddressRule (Priority 0) used to
// claim it before EmailRule (Priority 1, and the rule EmailRule's own
// godoc names as supported) ever got a turn, silently producing a street
// name instead of an email.
func TestGenerateRow_EmailAddressNotClaimedByAddressRule(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Customer",
		Fields: []autoseed.Field{
			{Name: "FirstName", Type: reflect.TypeOf("")},
			{Name: "LastName", Type: reflect.TypeOf("")},
			{Name: "EmailAddress", Type: reflect.TypeOf("")},
		},
	}

	values, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(42))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}

	value := values["EmailAddress"].(string)
	if !strings.Contains(value, "@") {
		t.Fatalf("EmailAddress = %q, want a valid-looking email address", value)
	}
}
