package inference_test

import (
	"errors"
	"reflect"
	"testing"
	"unicode/utf8"

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

// TestGenerateRow_TruncationIsRuneAwareNotByteAware guards a real bug:
// truncation cut at a byte index, not a rune index. Field.Size models a
// SQL VARCHAR(n) character limit, and a byte-index cut through a
// multi-byte UTF-8 rune produces a value that is both over the character
// limit by the database's own counting and not even valid UTF-8.
func TestGenerateRow_TruncationIsRuneAwareNotByteAware(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "City", value: "Curaçao"})
	entity := autoseed.Entity{
		Name:   "Address",
		Fields: []autoseed.Field{{Name: "City", Type: reflect.TypeOf(""), Size: 5}},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	got := values["City"].(string)
	if !utf8.ValidString(got) {
		t.Fatalf("City = %q is not valid UTF-8 — a byte-index cut split the ç in half", got)
	}
	if want := "Curaç"; got != want {
		t.Fatalf("City = %q, want %q (first 5 runes of \"Curaçao\")", got, want)
	}
}

// TestGenerateRow_GeneratesNaturalNonAutoIncrementPrimaryKeyField guards a
// real bug: GenerateRow used to skip every PrimaryKey field
// unconditionally, including one that is neither auto-increment (so the
// database won't generate it) nor a foreign key (so persistence won't
// copy it from a parent). A composite key mixing a foreign key column
// with one plain, natural column — e.g. WarehouseID+ProductID+
// EffectiveDate, EffectiveDate not covered by any reference — left that
// column at its permanent zero value, making every row for the same
// (WarehouseID, ProductID) pair collide on it too.
func TestGenerateRow_GeneratesNaturalNonAutoIncrementPrimaryKeyField(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "EffectiveDate", value: "generated"})
	entity := autoseed.Entity{
		Name: "PriceHistory",
		Fields: []autoseed.Field{
			{Name: "WarehouseID", Type: reflect.TypeOf(0), PrimaryKey: true},
			{Name: "EffectiveDate", Type: reflect.TypeOf(""), PrimaryKey: true},
		},
		References: []autoseed.Reference{{Fields: []string{"WarehouseID"}, Target: "Warehouse"}},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if _, ok := values["WarehouseID"]; ok {
		t.Fatal("WarehouseID is a foreign key, must not be generated")
	}
	if values["EffectiveDate"] != "generated" {
		t.Fatalf("EffectiveDate = %v, want a generated value: it is a primary key field but neither auto-increment nor a foreign key, so nothing else would ever give it one", values["EffectiveDate"])
	}
}

func TestGenerateRow_SkipsPrimaryKeyAndForeignKeyFields(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "Name", value: "x"})
	entity := autoseed.Entity{
		Name: "Order",
		Fields: []autoseed.Field{
			{Name: "ID", Type: reflect.TypeOf(0), PrimaryKey: true, AutoIncrement: true},
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

// TestGenerateRow_NilRateLeavesNullableFieldsOutEntirely guards the core
// null-rate mechanism: at rate 1.0, every Nullable field comes back nil
// -- never reaching its matched rule at all -- while a non-Nullable field
// on the same entity is untouched.
func TestGenerateRow_NilRateLeavesNullableFieldsOutEntirely(t *testing.T) {
	gen := inference.NewGenerator(
		fixedRule{priority: 0, claims: "Bio", value: "should never be seen"},
		fixedRule{priority: 0, claims: "Name", value: "always present"},
	).WithNilRate(1)
	entity := autoseed.Entity{
		Name: "Widget",
		Fields: []autoseed.Field{
			{Name: "Bio", Type: reflect.TypeOf(""), Nullable: true},
			{Name: "Name", Type: reflect.TypeOf(""), Nullable: false},
		},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if values["Bio"] != nil {
		t.Fatalf("Bio = %v, want nil at nil rate 1.0", values["Bio"])
	}
	if values["Name"] != "always present" {
		t.Fatalf("Name = %v, want %q: a non-Nullable field must never be left out", values["Name"], "always present")
	}
}

// TestGenerateRow_ZeroNilRateNeverLeavesFieldsOut guards the default:
// with no WithNilRate call (rate 0), a Nullable field always gets a
// generated value, matching every pre-existing test's expectation.
func TestGenerateRow_ZeroNilRateNeverLeavesFieldsOut(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "Bio", value: "present"})
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Bio", Type: reflect.TypeOf(""), Nullable: true}},
	}

	for seed := uint64(0); seed < 50; seed++ {
		values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(seed))
		if err != nil {
			t.Fatalf("seed %d: GenerateRow: %v", seed, err)
		}
		if values["Bio"] != "present" {
			t.Fatalf("seed %d: Bio = %v, want %q at the default nil rate", seed, values["Bio"], "present")
		}
	}
}

// TestGenerateRow_NilRateNeverAppliesToSoftDeleteField guards that a
// field the adapter flagged SoftDelete is excluded from the generic
// null-rate roll: SoftDeleteRule already owns that field's own,
// CreatedAt/UpdatedAt-coherent null-vs-not split, which a blind rate-1.0
// roll would bypass entirely.
func TestGenerateRow_NilRateNeverAppliesToSoftDeleteField(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "DeletedAt", value: "from the real rule"}).WithNilRate(1)
	entity := autoseed.Entity{
		Name: "Widget",
		Fields: []autoseed.Field{
			{Name: "DeletedAt", Type: reflect.TypeOf(""), Nullable: true, SoftDelete: true},
		},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if values["DeletedAt"] != "from the real rule" {
		t.Fatalf("DeletedAt = %v, want the SoftDelete-aware rule's own value, not the generic nil-rate roll", values["DeletedAt"])
	}
}

// TestGenerateRow_NilRateDeterministic guards that which rows land on nil
// at a given nil rate is itself seed-derived and repeatable, the same
// determinism guarantee every other draw in this pipeline carries.
func TestGenerateRow_NilRateDeterministic(t *testing.T) {
	gen := inference.NewGenerator(fixedRule{priority: 0, claims: "Bio", value: "present"}).WithNilRate(0.5)
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Bio", Type: reflect.TypeOf(""), Nullable: true}},
	}
	root := autoseed.NewSeededSource(7)

	build := func() []bool {
		results := make([]bool, 100)
		for i := range results {
			values, err := gen.GenerateRow(entity, root.Entity("Widget").Row(i))
			if err != nil {
				t.Fatalf("row %d: GenerateRow: %v", i, err)
			}
			results[i] = values["Bio"] == nil
		}
		return results
	}

	first := build()
	sawNil, sawValue := false, false
	for _, isNil := range first {
		if isNil {
			sawNil = true
		} else {
			sawValue = true
		}
	}
	if !sawNil || !sawValue {
		t.Fatalf("100 rows at nil rate 0.5 produced no mix of nil/non-nil, want both: %v", first)
	}

	for i := 0; i < 20; i++ {
		if got := build(); !equalBoolSlices(got, first) {
			t.Fatalf("run %d: nil pattern differs from the first run", i)
		}
	}
}

// TestGenerateRow_NilRateApproximatesConfiguredRate guards actual
// proportionality, not just "some mix of nil and non-nil": a roll that
// ignored the configured rate value entirely (an on/off switch, or a
// fixed ~50/50 split regardless of rate) would still pass a weaker
// "both values appeared" check. Mirrors the band-check style
// softdelete_test.go already uses for its own 90/10 split.
func TestGenerateRow_NilRateApproximatesConfiguredRate(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Bio", Type: reflect.TypeOf(""), Nullable: true}},
	}
	const rows = 1000

	for _, rate := range []float64{0.1, 0.5, 0.9} {
		gen := inference.NewGenerator(fixedRule{priority: 0, claims: "Bio", value: "present"}).WithNilRate(rate)
		root := autoseed.NewSeededSource(7)

		nilCount := 0
		for i := 0; i < rows; i++ {
			values, err := gen.GenerateRow(entity, root.Entity("Widget").Row(i))
			if err != nil {
				t.Fatalf("rate %v row %d: GenerateRow: %v", rate, i, err)
			}
			if values["Bio"] == nil {
				nilCount++
			}
		}

		got := float64(nilCount) / rows
		if got < rate-0.1 || got > rate+0.1 {
			t.Fatalf("rate %v: observed nil fraction %.3f over %d rows, want within 0.1 of the configured rate", rate, got, rows)
		}
	}
}

// TestGenerateRow_NilRateNeverAppliesToForeignKeyField guards that the
// pre-existing skip[] guard (foreign key columns are never generated —
// persistence copies them from the parent) still wins over a Nullable
// field also being a foreign key: gormseed can mark a pointer-typed FK
// column Nullable, so this combination is real, not hypothetical.
func TestGenerateRow_NilRateNeverAppliesToForeignKeyField(t *testing.T) {
	gen := inference.NewGenerator().WithNilRate(1)
	entity := autoseed.Entity{
		Name: "Widget",
		Fields: []autoseed.Field{
			{Name: "OwnerID", Type: reflect.TypeOf(0), Nullable: true},
		},
		References: []autoseed.Reference{
			{Fields: []string{"OwnerID"}, Target: "Owner", Nullable: true},
		},
	}

	values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if _, present := values["OwnerID"]; present {
		t.Fatalf("OwnerID = %v, want absent from the row entirely: a foreign key is never generated, regardless of Nullable or nil rate", values["OwnerID"])
	}
}

// coinFlipRule mimics SoftDeleteRule's own idiom (seed.Rand() first,
// then a Float64() threshold) to prove the null-rate pre-pass's roll
// doesn't entangle a surviving field's own randomness with the roll that
// decided it would survive.
type coinFlipRule struct{ claims string }

func (r coinFlipRule) Priority() int { return 0 }

func (r coinFlipRule) CanInfer(field autoseed.Field) bool { return field.Name == r.claims }

func (r coinFlipRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return seed.Rand().Float64() < 0.5
}

// TestGenerateRow_NilRateDoesNotEntangleSurvivingFieldsOwnRandomness
// guards a real bug found in review: the null-rate pre-pass used to roll
// its decision from the exact same SeededSource node
// (seed.Field(field.Name)) a surviving field's own rule then received
// unchanged. Since SeededSource derivation is pure (no advancing state),
// a rule using the same seed.Rand()-first idiom SoftDeleteRule does got
// the identical draw the pre-pass already consumed — deterministically
// entangling the rule's own decision with the roll that decided it
// would survive, up to fully collapsing a 50/50 rule to one constant
// outcome. At nil rate 0.9, only rows whose roll lands in [0.9, 1.0)
// survive; if the entanglement were still present, every surviving
// row's coinFlipRule result would come out identical (Float64() on the
// exact same derived seed the pre-pass just tested against 0.9 can
// never itself land below 0.5). With the fix, both outcomes must appear.
func TestGenerateRow_NilRateDoesNotEntangleSurvivingFieldsOwnRandomness(t *testing.T) {
	gen := inference.NewGenerator(coinFlipRule{claims: "Flag"}).WithNilRate(0.9)
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Flag", Type: reflect.TypeOf(false), Nullable: true}},
	}
	root := autoseed.NewSeededSource(11)

	sawTrue, sawFalse := false, false
	for i := 0; i < 500; i++ {
		values, err := gen.GenerateRow(entity, root.Entity("Widget").Row(i))
		if err != nil {
			t.Fatalf("row %d: GenerateRow: %v", i, err)
		}
		result, ok := values["Flag"].(bool)
		if !ok {
			continue // this row was nulled by the pre-pass, not relevant here
		}
		if result {
			sawTrue = true
		} else {
			sawFalse = true
		}
	}
	if !sawTrue || !sawFalse {
		t.Fatal("every surviving row's own 50/50 rule collapsed to a single outcome, want a genuine mix: the null-rate roll and the rule's own randomness must be independent")
	}
}

// TestGenerateRow_NilRateDoesNotBiasSurvivingNumericValues guards the
// distributional half of the same bug: a rule that builds its value
// directly from the field's own seed (as PriceRule and genericNumberRule
// do via faker(seed)) used to have that value mathematically constrained
// to the top (1-nilRate) fraction of its range, since surviving required
// the identical draw to land above the nil-rate threshold. At nil rate
// 0.5, a value below the midpoint of the rule's declared range must
// still be reachable among surviving rows.
func TestGenerateRow_NilRateDoesNotBiasSurvivingNumericValues(t *testing.T) {
	gen := inference.NewGenerator(fixedFloatRule{claims: "Weight"}).WithNilRate(0.5)
	entity := autoseed.Entity{
		Name:   "Widget",
		Fields: []autoseed.Field{{Name: "Weight", Type: reflect.TypeOf(float64(0)), Nullable: true}},
	}
	root := autoseed.NewSeededSource(3)

	sawBelowMidpoint := false
	for i := 0; i < 500; i++ {
		values, err := gen.GenerateRow(entity, root.Entity("Widget").Row(i))
		if err != nil {
			t.Fatalf("row %d: GenerateRow: %v", i, err)
		}
		value, ok := values["Weight"].(float64)
		if !ok {
			continue
		}
		if value < 500 {
			sawBelowMidpoint = true
			break
		}
	}
	if !sawBelowMidpoint {
		t.Fatal("no surviving row's Weight fell below the midpoint of its [0,1000) range, want the null-rate roll independent of the rule's own value")
	}
}

// fixedFloatRule mimics a numeric rule (PriceRule, genericNumberRule)
// that draws its value straight from the field's own seed, the shape
// that exposed the seed-entanglement bug.
type fixedFloatRule struct{ claims string }

func (r fixedFloatRule) Priority() int { return 0 }

func (r fixedFloatRule) CanInfer(field autoseed.Field) bool { return field.Name == r.claims }

func (r fixedFloatRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return seed.Rand().Float64() * 1000
}

func equalBoolSlices(a, b []bool) bool {
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
