package autoseed_test

import (
	"reflect"
	"testing"

	"github.com/danellalc/autoseed"
)

func coveragePlanFor(t *testing.T, entities []autoseed.Entity) *autoseed.GenerationPlan {
	t.Helper()
	graph, err := autoseed.NewDependencyGraph(entities)
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}
	result, err := graph.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan, err := autoseed.PlanCoverage(entities, result.Order, result.Deferred)
	if err != nil {
		t.Fatalf("PlanCoverage: %v", err)
	}
	return plan
}

func TestPlanCoverage_PlainRootEntityGetsOneRow(t *testing.T) {
	plan := coveragePlanFor(t, []autoseed.Entity{{Name: "Customer"}})
	p := entityPlan(t, plan, "Customer")
	if p.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1: a root entity with no coverage axis needs nothing more than existing", p.RowCount)
	}
}

// TestPlanCoverage_DriverTargetGetsThreeRowsRegardlessOfOwnAxes guards
// that an entity gets at least 3 rows the moment something depends on
// it, even if the entity itself has no Nullable/bool/sized-string field
// of its own — zero/one/many needs somewhere to land.
func TestPlanCoverage_DriverTargetGetsThreeRowsRegardlessOfOwnAxes(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer"},
		{Name: "Order", References: []autoseed.Reference{{Fields: []string{"CustomerID"}, Target: "Customer", Nullable: false}}},
	}
	plan := coveragePlanFor(t, entities)
	customer := entityPlan(t, plan, "Customer")
	if customer.RowCount != 3 {
		t.Fatalf("Customer.RowCount = %d, want 3 (drives Order's zero/one/many coverage)", customer.RowCount)
	}
}

func TestPlanCoverage_NullableFieldNeedsTwoRows(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer", Fields: []autoseed.Field{{Name: "Bio", Type: reflect.TypeOf(""), Nullable: true}}},
	}
	plan := coveragePlanFor(t, entities)
	p := entityPlan(t, plan, "Customer")
	if p.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2 (one null, one with a value)", p.RowCount)
	}
}

func TestPlanCoverage_BoolFieldNeedsTwoRows(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer", Fields: []autoseed.Field{{Name: "Active", Type: reflect.TypeOf(false)}}},
	}
	plan := coveragePlanFor(t, entities)
	p := entityPlan(t, plan, "Customer")
	if p.RowCount != 2 {
		t.Fatalf("RowCount = %d, want 2 (true and false)", p.RowCount)
	}
}

func TestPlanCoverage_SizedStringFieldNeedsThreeRows(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer", Fields: []autoseed.Field{{Name: "Code", Type: reflect.TypeOf(""), Size: 10}}},
	}
	plan := coveragePlanFor(t, entities)
	p := entityPlan(t, plan, "Customer")
	if p.RowCount != 3 {
		t.Fatalf("RowCount = %d, want 3 (empty, one character, and Size characters)", p.RowCount)
	}
}

func TestPlanCoverage_UnsizedStringFieldNeedsNoExtraRows(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer", Fields: []autoseed.Field{{Name: "Notes", Type: reflect.TypeOf(""), Size: 0}}},
	}
	plan := coveragePlanFor(t, entities)
	p := entityPlan(t, plan, "Customer")
	if p.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1: an unsized string field has no declared boundary to cover", p.RowCount)
	}
}

// TestPlanCoverage_DependentChildCountsCoverZeroOneMany guards the
// actual zero/one/many distribution: Order's driver (Customer) gets 3
// rows, and Order's own ChildCounts must be exactly [0, 1, 2].
func TestPlanCoverage_DependentChildCountsCoverZeroOneMany(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer"},
		{Name: "Order", References: []autoseed.Reference{{Fields: []string{"CustomerID"}, Target: "Customer", Nullable: false}}},
	}
	plan := coveragePlanFor(t, entities)
	order := entityPlan(t, plan, "Order")
	want := []int{0, 1, 2}
	if len(order.ChildCounts) != len(want) {
		t.Fatalf("ChildCounts = %v, want %v", order.ChildCounts, want)
	}
	for i := range want {
		if order.ChildCounts[i] != want[i] {
			t.Fatalf("ChildCounts = %v, want %v", order.ChildCounts, want)
		}
	}
	if order.RowCount != 3 {
		t.Fatalf("Order.RowCount = %d, want 3 (0+1+2)", order.RowCount)
	}
}

// TestPlanCoverage_SharedPrimaryKeyDegradesToZeroOneOne guards that the
// SAME shared-key cap PlanGeneration enforces for the bulk path also
// clamps PlanCoverage's [0,1,2] pattern down to [0,1,1] -- "many" is
// structurally impossible for a shared-primary-key one-to-one, and
// PlanCoverage must not silently violate that cap just because its
// child-count strategy differs from the bulk path's.
func TestPlanCoverage_SharedPrimaryKeyDegradesToZeroOneOne(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Product"},
		{
			Name:   "ProductProfile",
			Fields: []autoseed.Field{{Name: "ProductID", PrimaryKey: true}},
			References: []autoseed.Reference{
				{Fields: []string{"ProductID"}, Target: "Product", Nullable: false},
			},
		},
	}
	plan := coveragePlanFor(t, entities)
	profile := entityPlan(t, plan, "ProductProfile")
	for i, count := range profile.ChildCounts {
		if count > 1 {
			t.Fatalf("ChildCounts[%d] = %d, want at most 1: a shared primary key can never hold more than one child per driver row", i, count)
		}
	}
}

// TestPlanCoverage_JunctionCapRespected guards the same for a
// many-to-many-shaped junction: PlanCoverage's pattern must never exceed
// the product-of-participant-row-counts cap junctionCap computes.
func TestPlanCoverage_JunctionCapRespected(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Post"},
		{Name: "Tag"},
		{
			Name: "post_tags",
			Fields: []autoseed.Field{
				{Name: "PostID", PrimaryKey: true},
				{Name: "TagID", PrimaryKey: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"PostID"}, Target: "Post", Nullable: false},
				{Fields: []string{"TagID"}, Target: "Tag", Nullable: false},
			},
		},
	}
	plan := coveragePlanFor(t, entities)
	post := entityPlan(t, plan, "Post")
	tag := entityPlan(t, plan, "Tag")
	joinPlan := entityPlan(t, plan, "post_tags")

	otherRowCount := tag.RowCount
	if joinPlan.Driver != "Post" {
		otherRowCount = post.RowCount
	}
	for i, count := range joinPlan.ChildCounts {
		if count > otherRowCount {
			t.Fatalf("ChildCounts[%d] = %d, want at most %d (the non-driver side's own row count)", i, count, otherRowCount)
		}
	}
}

// TestPlanCoverage_RequiredNonDriverTargetNeverEndsAtZero guards that
// PlanCoverage still runs through the same required-target backstop
// PlanGeneration relies on: a required, non-driver reference target
// must never end up with zero rows just because PlanCoverage's own
// distribution happened to draw zero for it everywhere.
func TestPlanCoverage_RequiredNonDriverTargetNeverEndsAtZero(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "DiamondRoot"},
		{Name: "DiamondLeft", References: []autoseed.Reference{
			{Fields: []string{"DiamondRootID"}, Target: "DiamondRoot", Nullable: false},
		}},
		{Name: "DiamondRight", References: []autoseed.Reference{
			{Fields: []string{"DiamondRootID"}, Target: "DiamondRoot", Nullable: false},
		}},
		{Name: "DiamondMerge", References: []autoseed.Reference{
			{Fields: []string{"DiamondLeftID"}, Target: "DiamondLeft", Nullable: false},
			{Fields: []string{"DiamondRightID"}, Target: "DiamondRight", Nullable: false},
		}},
	}
	plan := coveragePlanFor(t, entities)
	merge := entityPlan(t, plan, "DiamondMerge")
	right := entityPlan(t, plan, "DiamondRight")
	if merge.RowCount > 0 && right.RowCount == 0 {
		t.Fatalf("DiamondMerge has %d rows but DiamondRight (a required, non-driver target) has 0 — every DiamondRightID would be orphaned", merge.RowCount)
	}
}

// TestPlanCoverage_DependentThatIsAlsoADriverGetsEnoughRowsForBoth guards
// the fix for a real bug: a dependent entity (has a required reference
// of its own) that is ALSO a driver target for something further
// downstream used to never go through coverageRowCount/fieldAxisRowCount
// at all — only a root entity did — so its own row count came purely
// from its driver's child-count pattern, capped by whatever shared-key
// cap applied, with no floor of its own. Product (root, drives
// ProductProfile) -> ProductProfile (shares Product's primary key, a
// one-to-one, and itself drives ProductProfileNote) -> ProductProfileNote
// (an ordinary required reference to ProductProfile). Before the fix,
// ProductProfile's shared-key cap alone gave it RowCount=2 (ChildCounts
// [0,1,1]), so ProductProfileNote's own coverageChildCounts(2) could
// never reach the "many" (2 children) pattern index at all. The floor
// must raise ProductProfile to enough rows for that to become possible.
func TestPlanCoverage_DependentThatIsAlsoADriverGetsEnoughRowsForBoth(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Product"},
		{
			Name:   "ProductProfile",
			Fields: []autoseed.Field{{Name: "ProductID", PrimaryKey: true}},
			References: []autoseed.Reference{
				{Fields: []string{"ProductID"}, Target: "Product", Nullable: false},
			},
		},
		{
			Name: "ProductProfileNote",
			References: []autoseed.Reference{
				{Fields: []string{"ProductProfileID"}, Target: "ProductProfile", Nullable: false},
			},
		},
	}
	plan := coveragePlanFor(t, entities)
	note := entityPlan(t, plan, "ProductProfileNote")

	sawMany := false
	for _, count := range note.ChildCounts {
		if count >= 2 {
			sawMany = true
		}
	}
	if !sawMany {
		t.Fatalf("ProductProfileNote.ChildCounts = %v, want at least one entry >= 2 (some ProductProfile row must host several notes)", note.ChildCounts)
	}
}

// TestPlanCoverage_DependentEntityOwnFieldAxisStillCoveredWhenCapped
// guards the field-axis half of the same fix: ProductProfile shares
// Product's primary key (capped to at most one child per Product row)
// and declares its own Size:10 string field. Before the fix,
// ProductProfile's RowCount came purely from the shared-key-capped
// child-count sum, never checked against its own fieldAxisRowCount, so
// the Size-characters boundary variant could silently never appear.
func TestPlanCoverage_DependentEntityOwnFieldAxisStillCoveredWhenCapped(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Product"},
		{
			Name: "ProductProfile",
			Fields: []autoseed.Field{
				{Name: "ProductID", PrimaryKey: true},
				{Name: "Description", Type: reflect.TypeOf(""), Size: 10},
			},
			References: []autoseed.Reference{
				{Fields: []string{"ProductID"}, Target: "Product", Nullable: false},
			},
		},
	}
	plan := coveragePlanFor(t, entities)
	profile := entityPlan(t, plan, "ProductProfile")
	if profile.RowCount < 3 {
		t.Fatalf("ProductProfile.RowCount = %d, want at least 3: its own Size:10 field needs empty/one-char/max-length rows to land somewhere", profile.RowCount)
	}
}

// TestPlanCoverage_JunctionRowCount guards that a many-to-many junction
// entity's own RowCount, not just its ChildCounts' per-entry cap, is
// exercised by a coverage plan — sum(ChildCounts) must actually reach
// the junction's own row count, not just stay under the per-row cap.
func TestPlanCoverage_JunctionRowCount(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Post"},
		{Name: "Tag"},
		{
			Name: "post_tags",
			Fields: []autoseed.Field{
				{Name: "PostID", PrimaryKey: true},
				{Name: "TagID", PrimaryKey: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"PostID"}, Target: "Post", Nullable: false},
				{Fields: []string{"TagID"}, Target: "Tag", Nullable: false},
			},
		},
	}
	plan := coveragePlanFor(t, entities)
	joinPlan := entityPlan(t, plan, "post_tags")
	total := 0
	for _, count := range joinPlan.ChildCounts {
		total += count
	}
	if joinPlan.RowCount != total {
		t.Fatalf("post_tags.RowCount = %d, want %d (sum of ChildCounts)", joinPlan.RowCount, total)
	}
	if joinPlan.RowCount == 0 {
		t.Fatal("post_tags.RowCount = 0, want at least some join rows to cover the association at all")
	}
}

func TestApplyCoverageOverrides_PrimaryKeyFieldNeverTouched(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Weird",
		Fields: []autoseed.Field{
			{Name: "ID", Type: reflect.TypeOf(0), PrimaryKey: true, Nullable: true},
		},
	}
	rows := []map[string]any{{"ID": 1}, {"ID": 2}}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[0]["ID"] != 1 || rows[1]["ID"] != 2 {
		t.Fatalf("a primary key field must never be touched here, got %v / %v", rows[0]["ID"], rows[1]["ID"])
	}
}

// TestApplyCoverageOverrides_UniqueBoolFieldOnlyOverridesFirstTwoRows
// guards the fix for a real bug: a bool has only two possible values, so
// forcing the same alternating true/false pattern past a third row
// deterministically manufactures a duplicate for a Unique bool field --
// one EnsureUnique cannot repair, since it only rewrites string-kind
// fields. Rows beyond the second must be left for GenerateRow's own
// value instead of being forced into a guaranteed collision.
func TestApplyCoverageOverrides_UniqueBoolFieldOnlyOverridesFirstTwoRows(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "FlagConfig",
		Fields: []autoseed.Field{{Name: "IsDefault", Type: reflect.TypeOf(false), Unique: true}},
	}
	rows := []map[string]any{
		{"IsDefault": nil},
		{"IsDefault": nil},
		{"IsDefault": "untouched"},
	}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[0]["IsDefault"] != false {
		t.Fatalf("rows[0][IsDefault] = %v, want false", rows[0]["IsDefault"])
	}
	if rows[1]["IsDefault"] != true {
		t.Fatalf("rows[1][IsDefault] = %v, want true", rows[1]["IsDefault"])
	}
	if rows[2]["IsDefault"] != "untouched" {
		t.Fatalf("rows[2][IsDefault] = %v, want left untouched (a third row would collide with row 0 or row 1)", rows[2]["IsDefault"])
	}
}

// TestApplyCoverageOverrides_SizedStringOneCharacterVariantsDiffer guards
// the fix for a real bug: with a declared Size of 1, the "one character"
// and "exactly Size characters" boundary states used to collapse to the
// identical string ("x" both times), guaranteeing a duplicate for a
// Unique, Size:1 string field that EnsureUnique has no room to repair
// (there is no character left over for a distinguishing suffix once the
// whole field is one character long).
func TestApplyCoverageOverrides_SizedStringOneCharacterVariantsDiffer(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Grade",
		Fields: []autoseed.Field{{Name: "Code", Type: reflect.TypeOf(""), Size: 1, Unique: true}},
	}
	rows := []map[string]any{{"Code": nil}, {"Code": nil}, {"Code": nil}}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[1]["Code"] == rows[2]["Code"] {
		t.Fatalf("rows[1][Code] and rows[2][Code] are both %q, want distinct one-character values", rows[1]["Code"])
	}
}

func TestApplyCoverageOverrides_NullableFieldAlternatesNilAndValue(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Customer",
		Fields: []autoseed.Field{{Name: "Bio", Type: reflect.TypeOf(""), Nullable: true}},
	}
	rows := []map[string]any{
		{"Bio": "hello"},
		{"Bio": "world"},
	}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[0]["Bio"] != nil {
		t.Fatalf("rows[0][Bio] = %v, want nil", rows[0]["Bio"])
	}
	if rows[1]["Bio"] != "world" {
		t.Fatalf("rows[1][Bio] = %v, want the generated value untouched", rows[1]["Bio"])
	}
}

func TestApplyCoverageOverrides_BoolFieldAlternatesTrueFalse(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Customer",
		Fields: []autoseed.Field{{Name: "Active", Type: reflect.TypeOf(false)}},
	}
	rows := []map[string]any{{"Active": nil}, {"Active": nil}}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[0]["Active"] != false {
		t.Fatalf("rows[0][Active] = %v, want false", rows[0]["Active"])
	}
	if rows[1]["Active"] != true {
		t.Fatalf("rows[1][Active] = %v, want true", rows[1]["Active"])
	}
}

func TestApplyCoverageOverrides_SizedStringFieldCyclesLengths(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Customer",
		Fields: []autoseed.Field{{Name: "Code", Type: reflect.TypeOf(""), Size: 5}},
	}
	rows := []map[string]any{{"Code": nil}, {"Code": nil}, {"Code": nil}}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[0]["Code"] != "" {
		t.Fatalf("rows[0][Code] = %q, want empty", rows[0]["Code"])
	}
	if got := rows[1]["Code"].(string); len(got) != 1 {
		t.Fatalf("rows[1][Code] = %q, want exactly 1 character", got)
	}
	if got := rows[2]["Code"].(string); len(got) != 5 {
		t.Fatalf("rows[2][Code] = %q, want exactly 5 characters (the field's Size)", got)
	}
}

// TestApplyCoverageOverrides_ReferenceFieldUntouched guards that a
// foreign key column is never overridden here, even when it's Nullable
// — its real value is assigned separately during persistence and
// overwrites whatever this leaves behind regardless, but touching it at
// all invites exactly the kind of adapter-specific surprise (a
// mutation's ClearField call on a field expecting a real value) this
// diff's own entseed work has already shown can bite.
func TestApplyCoverageOverrides_ReferenceFieldUntouched(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Order",
		Fields: []autoseed.Field{
			{Name: "CustomerID", Type: reflect.TypeOf(0), Nullable: true},
		},
		References: []autoseed.Reference{
			{Fields: []string{"CustomerID"}, Target: "Customer", Nullable: true},
		},
	}
	rows := []map[string]any{{"CustomerID": 42}, {"CustomerID": 43}}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[0]["CustomerID"] != 42 || rows[1]["CustomerID"] != 43 {
		t.Fatalf("a reference field must never be touched here, got %v / %v", rows[0]["CustomerID"], rows[1]["CustomerID"])
	}
}

// TestApplyCoverageOverrides_NullableTakesPriorityOverBool guards that a
// field which is both Nullable and bool-kind gets exactly one axis
// applied, not two competing writes to the same field: Nullable wins,
// since it's the axis the feature explicitly promises, and a later
// write must never silently undo an earlier one's.
func TestApplyCoverageOverrides_NullableTakesPriorityOverBool(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Customer",
		Fields: []autoseed.Field{{Name: "Verified", Type: reflect.TypeOf(false), Nullable: true}},
	}
	rows := []map[string]any{{"Verified": true}, {"Verified": true}}
	autoseed.ApplyCoverageOverrides(entity, rows)
	if rows[0]["Verified"] != nil {
		t.Fatalf("rows[0][Verified] = %v, want nil: Nullable must take priority over the bool axis", rows[0]["Verified"])
	}
}
