package autoseed_test

import (
	"errors"
	"testing"

	"github.com/danellalc/autoseed"
)

func planFor(t *testing.T, entities []autoseed.Entity, options autoseed.Options) *autoseed.GenerationPlan {
	t.Helper()
	graph, err := autoseed.NewDependencyGraph(entities)
	if err != nil {
		t.Fatalf("NewDependencyGraph: %v", err)
	}
	result, err := graph.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	plan, err := autoseed.PlanGeneration(entities, result.Order, result.Deferred, autoseed.NewSeededSource(options.Seed), options)
	if err != nil {
		t.Fatalf("PlanGeneration: %v", err)
	}
	return plan
}

func entityPlan(t *testing.T, plan *autoseed.GenerationPlan, name string) autoseed.EntityGenerationPlan {
	t.Helper()
	for _, p := range plan.Entities {
		if p.Entity == name {
			return p
		}
	}
	t.Fatalf("no plan for entity %q", name)
	return autoseed.EntityGenerationPlan{}
}

func TestPlanGeneration_RootEntityGetsScale(t *testing.T) {
	plan := planFor(t, []autoseed.Entity{{Name: "Customer"}}, autoseed.NewOptions(autoseed.WithScale(250)))
	p := entityPlan(t, plan, "Customer")
	if p.RowCount != 250 {
		t.Fatalf("RowCount = %d, want 250", p.RowCount)
	}
	if p.Driver != "" {
		t.Fatalf("Driver = %q, want empty for a root entity", p.Driver)
	}
}

func TestPlanGeneration_DependentEntityScalesFromDriver(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer"},
		{Name: "Order", References: []autoseed.Reference{{Fields: []string{"CustomerID"}, Target: "Customer", Nullable: false}}},
	}
	plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithScale(50)))

	customer := entityPlan(t, plan, "Customer")
	order := entityPlan(t, plan, "Order")

	if order.Driver != "Customer" {
		t.Fatalf("Order.Driver = %q, want Customer", order.Driver)
	}
	if len(order.ChildCounts) != customer.RowCount {
		t.Fatalf("len(ChildCounts) = %d, want %d (one draw per Customer row)", len(order.ChildCounts), customer.RowCount)
	}
	sum := 0
	for _, c := range order.ChildCounts {
		if c < 0 {
			t.Fatalf("child count %d is negative", c)
		}
		sum += c
	}
	if order.RowCount != sum {
		t.Fatalf("RowCount = %d, want sum of ChildCounts = %d", order.RowCount, sum)
	}
}

func TestPlanGeneration_LongTailProducesVarietyIncludingZero(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer"},
		{Name: "Order", References: []autoseed.Reference{{Fields: []string{"CustomerID"}, Target: "Customer", Nullable: false}}},
	}
	plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithScale(500)))
	order := entityPlan(t, plan, "Order")

	sawZero, sawMany := false, false
	for _, c := range order.ChildCounts {
		if c == 0 {
			sawZero = true
		}
		if c >= 10 {
			sawMany = true
		}
	}
	if !sawZero {
		t.Fatal("500 draws never produced a zero-child row, want the exponential draw to occasionally hit zero")
	}
	if !sawMany {
		t.Fatal("500 draws never produced a row with 10+ children, want a long tail")
	}
}

func TestPlanGeneration_DeferredEdgeNeverDrives(t *testing.T) {
	entities := []autoseed.Entity{
		{
			Name: "Employee",
			References: []autoseed.Reference{
				{Fields: []string{"ManagerID"}, Target: "Employee", Nullable: true},
			},
		},
	}
	plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithScale(30)))
	employee := entityPlan(t, plan, "Employee")

	if employee.Driver != "" {
		t.Fatalf("Driver = %q, want empty: a deferred (nullable, cycle-broken) reference must never drive cardinality", employee.Driver)
	}
	if employee.RowCount != 30 {
		t.Fatalf("RowCount = %d, want 30 (root scale)", employee.RowCount)
	}
}

func TestPlanGeneration_OnlyOneSideOfManyToManyDrives(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Post"},
		{Name: "Tag"},
		{
			Name: "post_tags",
			References: []autoseed.Reference{
				{Fields: []string{"PostID"}, Target: "Post", Nullable: false},
				{Fields: []string{"TagID"}, Target: "Tag", Nullable: false},
			},
		},
	}
	plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithScale(20)))
	joinPlan := entityPlan(t, plan, "post_tags")

	if joinPlan.Driver != "Post" {
		t.Fatalf("Driver = %q, want Post (sorts before Tag)", joinPlan.Driver)
	}
}

// TestPlanGeneration_NonDriverRequiredTargetNeverEndsAtZero guards a real
// bug a property test against gormseed.Seed found: DiamondMerge requires
// both DiamondLeft (its driver) and DiamondRight (round-robin, not the
// driver). DiamondRight's own row count comes from an independent
// long-tail draw off DiamondRoot and can legitimately land on zero — and
// when it did, DiamondMerge silently left DiamondRightID at its Go zero
// value, an orphaned foreign key, with no error. seed=2, scale=1 is the
// exact case that reproduced it before this fix.
func TestPlanGeneration_NonDriverRequiredTargetNeverEndsAtZero(t *testing.T) {
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

	for seed := uint64(0); seed < 200; seed++ {
		plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(seed), autoseed.WithScale(1)))
		merge := entityPlan(t, plan, "DiamondMerge")
		right := entityPlan(t, plan, "DiamondRight")

		if merge.RowCount > 0 && right.RowCount == 0 {
			t.Fatalf("seed %d: DiamondMerge has %d rows but DiamondRight (a required, non-driver target) has 0 — every DiamondRightID would be orphaned", seed, merge.RowCount)
		}
	}
}

// TestPlanGeneration_JunctionCapsChildCountAtNonDriverRowCount guards a
// real bug: InventoryItem-shaped entities (composite primary key made of
// exactly the driver's and one other required reference's foreign key
// columns — the shape of a many-to-many join table too) drew each driver
// row's child count from the same unbounded long-tail distribution as any
// other dependent entity. Whenever a draw exceeded the non-driver
// target's own row count, gormseed.Seed could only satisfy it by reusing
// a non-driver row within the same driver row's block, colliding on the
// entity's own primary key at insert time. seed=185, scale=2 on this
// exact Warehouse/Product/InventoryItem shape reproduced a raw SQLite
// UNIQUE constraint violation before this fix.
func TestPlanGeneration_JunctionCapsChildCountAtNonDriverRowCount(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Warehouse"},
		{Name: "Product"},
		{
			Name: "InventoryItem",
			Fields: []autoseed.Field{
				{Name: "WarehouseID", PrimaryKey: true},
				{Name: "ProductID", PrimaryKey: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"WarehouseID"}, Target: "Warehouse", Nullable: false},
				{Fields: []string{"ProductID"}, Target: "Product", Nullable: false},
			},
		},
	}

	for seed := uint64(0); seed < 300; seed++ {
		plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(seed), autoseed.WithScale(2)))
		item := entityPlan(t, plan, "InventoryItem")
		warehouse := entityPlan(t, plan, "Warehouse")

		if len(item.JunctionParticipants) != 1 || item.JunctionParticipants[0].Target != "Warehouse" {
			t.Fatalf("seed %d: JunctionParticipants = %v, want exactly one participant targeting Warehouse", seed, item.JunctionParticipants)
		}
		for i, count := range item.ChildCounts {
			if count > warehouse.RowCount {
				t.Fatalf("seed %d: Product row %d wants %d InventoryItem children, more than the %d Warehouse rows available to pair with — every pairing must be a distinct (Warehouse, Product), so this can never be satisfied without a duplicate", seed, i, count, warehouse.RowCount)
			}
		}
	}
}

// TestPlanGeneration_NonJunctionSharedShapeUncapped guards against the
// junction cap firing on an entity that only superficially resembles one:
// two required references but its own primary key is a plain surrogate
// ID, not composed of those references' fields. Two order lines for the
// same product on the same order are a legitimate, uncapped shape.
func TestPlanGeneration_NonJunctionSharedShapeUncapped(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Order"},
		{Name: "Product"},
		{
			Name: "OrderItem",
			Fields: []autoseed.Field{
				{Name: "ID", PrimaryKey: true, AutoIncrement: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"OrderID"}, Target: "Order", Nullable: false},
				{Fields: []string{"ProductID"}, Target: "Product", Nullable: false},
			},
		},
	}

	plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(1), autoseed.WithScale(3)))
	item := entityPlan(t, plan, "OrderItem")
	if len(item.JunctionParticipants) != 0 {
		t.Fatalf("JunctionParticipants = %v, want empty: OrderItem's primary key is its own surrogate ID, not the two references", item.JunctionParticipants)
	}
}

// TestPlanGeneration_SharedPrimaryKeyCapsAtOnePerDriverRow guards the
// GORM equivalent of EF Core's table splitting: ProductProfile.ProductID
// is both its own primary key and its only foreign key to Product. A
// driver row's foreign key value is copied verbatim into the child's own
// primary key, so a second child for the same driver row would collide
// on it — the long-tail draw must never exceed 1 per driver row here.
func TestPlanGeneration_SharedPrimaryKeyCapsAtOnePerDriverRow(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Product"},
		{
			Name: "ProductProfile",
			Fields: []autoseed.Field{
				{Name: "ProductID", PrimaryKey: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"ProductID"}, Target: "Product", Nullable: false},
			},
		},
	}

	for seed := uint64(0); seed < 300; seed++ {
		plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(seed), autoseed.WithScale(3)))
		profile := entityPlan(t, plan, "ProductProfile")
		for i, count := range profile.ChildCounts {
			if count > 1 {
				t.Fatalf("seed %d: Product row %d wants %d ProductProfile children, want at most 1 (its ProductID is both PK and FK)", seed, i, count)
			}
		}
	}
}

func TestPlanGeneration_NegativeScaleReturnsError(t *testing.T) {
	_, err := autoseed.PlanGeneration(
		[]autoseed.Entity{{Name: "Customer"}},
		[]string{"Customer"},
		nil,
		autoseed.NewSeededSource(1),
		autoseed.NewOptions(autoseed.WithScale(-1)),
	)
	if !errors.Is(err, autoseed.ErrInvalidScale) {
		t.Fatalf("got %v, want ErrInvalidScale", err)
	}
}

func TestPlanGeneration_NilSeedReturnsError(t *testing.T) {
	_, err := autoseed.PlanGeneration(
		[]autoseed.Entity{{Name: "Customer"}},
		[]string{"Customer"},
		nil,
		nil,
		autoseed.NewOptions(),
	)
	if !errors.Is(err, autoseed.ErrNilSeed) {
		t.Fatalf("got %v, want ErrNilSeed", err)
	}
}

// TestPlanGeneration_SelfReferencingJunctionCapsAndDisambiguates guards a
// real bug: a self-referencing many-to-many join (both foreign keys
// targeting the same entity, e.g. a "Person follows Person" friendship
// table) has two required references with an identical Target, so
// matching by Target alone could never tell them apart. junctionCap must
// still recognize the pair by their distinct Fields and cap accordingly.
func TestPlanGeneration_SelfReferencingJunctionCapsAndDisambiguates(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Person"},
		{
			Name: "PersonFollow",
			Fields: []autoseed.Field{
				{Name: "FollowerID", PrimaryKey: true},
				{Name: "FollowingID", PrimaryKey: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"FollowerID"}, Target: "Person", Nullable: false},
				{Fields: []string{"FollowingID"}, Target: "Person", Nullable: false},
			},
		},
	}

	for seed := uint64(0); seed < 200; seed++ {
		plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(seed), autoseed.WithScale(3)))
		follow := entityPlan(t, plan, "PersonFollow")
		person := entityPlan(t, plan, "Person")

		driverKey := joinFields(follow.DriverFields)
		if driverKey == "" || len(follow.JunctionParticipants) != 1 {
			t.Fatalf("seed %d: Driver/Junction fields not both set: driver=%v junction=%v", seed, follow.DriverFields, follow.JunctionParticipants)
		}
		junctionKey := joinFields(follow.JunctionParticipants[0].Fields)
		if driverKey == junctionKey {
			t.Fatalf("seed %d: DriverFields and the junction participant's Fields are identical (%v) — the two references were not disambiguated, both would copy the same parent row", seed, follow.DriverFields)
		}
		maxValidPartners := person.RowCount - 1 // a row can never pair with itself
		for i, count := range follow.ChildCounts {
			if count > maxValidPartners {
				t.Fatalf("seed %d: Person row %d wants %d PersonFollow children, more than the %d other Person rows available to pair with (excluding itself)", seed, i, count, maxValidPartners)
			}
		}
	}
}

// TestPlanGeneration_JunctionCapAppliesDespiteExtraRequiredReference
// guards a real bug: junctionCap used to bail on any entity with more
// than two required references, even when exactly two of them still
// exactly cover the primary key and the third is unrelated to it. Adding
// an ordinary required foreign key to an otherwise-correctly-shaped
// attributed-join entity must not silently remove its cap.
func TestPlanGeneration_JunctionCapAppliesDespiteExtraRequiredReference(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Warehouse"},
		{Name: "Product"},
		{Name: "StorageZone"},
		{
			Name: "InventoryItem",
			Fields: []autoseed.Field{
				{Name: "WarehouseID", PrimaryKey: true},
				{Name: "ProductID", PrimaryKey: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"WarehouseID"}, Target: "Warehouse", Nullable: false},
				{Fields: []string{"ProductID"}, Target: "Product", Nullable: false},
				{Fields: []string{"StorageZoneID"}, Target: "StorageZone", Nullable: false},
			},
		},
	}

	for seed := uint64(0); seed < 200; seed++ {
		plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(seed), autoseed.WithScale(2)))
		item := entityPlan(t, plan, "InventoryItem")
		if len(item.JunctionParticipants) == 0 {
			t.Fatalf("seed %d: JunctionParticipants empty despite Warehouse+Product exactly covering the primary key — the extra StorageZone reference must not disqualify the pair that does", seed)
		}
	}
}

// TestPlanGeneration_TernaryJunctionCapsAtProductOfParticipants guards the
// N-ary generalization of the junction cap: StudentCourseTeacher's
// primary key is exactly its driver reference plus TWO other required
// references together, not just one. selectDriver picks Course (its
// Target sorts first alphabetically among Course/Student/Teacher), so
// Student and Teacher become the junction participants. Every driver
// row's child count must never exceed the PRODUCT of Student's and
// Teacher's own row counts — asking for more would demand more distinct
// (Student, Teacher) combinations than exist to pair with.
func TestPlanGeneration_TernaryJunctionCapsAtProductOfParticipants(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Student"},
		{Name: "Course"},
		{Name: "Teacher"},
		{
			Name: "StudentCourseTeacher",
			Fields: []autoseed.Field{
				{Name: "StudentID", PrimaryKey: true},
				{Name: "CourseID", PrimaryKey: true},
				{Name: "TeacherID", PrimaryKey: true},
			},
			References: []autoseed.Reference{
				{Fields: []string{"StudentID"}, Target: "Student", Nullable: false},
				{Fields: []string{"CourseID"}, Target: "Course", Nullable: false},
				{Fields: []string{"TeacherID"}, Target: "Teacher", Nullable: false},
			},
		},
	}

	for seed := uint64(0); seed < 200; seed++ {
		plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(seed), autoseed.WithScale(5)))
		assignment := entityPlan(t, plan, "StudentCourseTeacher")
		if assignment.Driver != "Course" {
			t.Fatalf("seed %d: Driver = %q, want Course (sorts first alphabetically)", seed, assignment.Driver)
		}
		student := entityPlan(t, plan, "Student")
		teacher := entityPlan(t, plan, "Teacher")

		if len(assignment.JunctionParticipants) != 2 {
			t.Fatalf("seed %d: JunctionParticipants = %v, want exactly 2 (Student, Teacher)", seed, assignment.JunctionParticipants)
		}
		if assignment.JunctionParticipants[0].Target != "Student" || assignment.JunctionParticipants[1].Target != "Teacher" {
			t.Fatalf("seed %d: JunctionParticipants targets = [%s %s], want [Student Teacher]",
				seed, assignment.JunctionParticipants[0].Target, assignment.JunctionParticipants[1].Target)
		}

		maxPerDriverRow := student.RowCount * teacher.RowCount
		for i, count := range assignment.ChildCounts {
			if count > maxPerDriverRow {
				t.Fatalf("seed %d: Course row %d wants %d children, more than the %d distinct (Student, Teacher) combinations available", seed, i, count, maxPerDriverRow)
			}
		}
	}
}

// TestPlanGeneration_JunctionIndicesTernaryProducesDistinctCombinations
// guards the mixed-radix index assignment JunctionIndices computes:
// every row within one driver block must draw a distinct (Course,
// Teacher) combination, and it must be confined within each target's own
// row count.
func TestPlanGeneration_JunctionIndicesTernaryProducesDistinctCombinations(t *testing.T) {
	participants := []autoseed.JunctionParticipant{
		{Target: "Course", Fields: []string{"CourseID"}},
		{Target: "Teacher", Fields: []string{"TeacherID"}},
	}
	rowsOf := func(target string) int {
		switch target {
		case "Course":
			return 4
		case "Teacher":
			return 3
		}
		return 0
	}

	driverRow := 2
	seen := make(map[[2]int]bool)
	for blockLocal := 0; blockLocal < 12; blockLocal++ {
		indices := autoseed.JunctionIndices(participants, "Student", driverRow, blockLocal, rowsOf)
		if len(indices) != 2 {
			t.Fatalf("blockLocal %d: len(indices) = %d, want 2", blockLocal, len(indices))
		}
		courseIdx, teacherIdx := indices[0], indices[1]
		if courseIdx < 0 || courseIdx >= 4 {
			t.Fatalf("blockLocal %d: courseIdx = %d, want in [0,4)", blockLocal, courseIdx)
		}
		if teacherIdx < 0 || teacherIdx >= 3 {
			t.Fatalf("blockLocal %d: teacherIdx = %d, want in [0,3)", blockLocal, teacherIdx)
		}
		key := [2]int{courseIdx, teacherIdx}
		if seen[key] {
			t.Fatalf("blockLocal %d: combination (Course=%d, Teacher=%d) repeats within the same driver block", blockLocal, courseIdx, teacherIdx)
		}
		seen[key] = true
	}
	if len(seen) != 12 {
		t.Fatalf("saw %d distinct combinations, want all 12 of the 4x3 grid", len(seen))
	}
}

// TestPlanGeneration_JunctionIndicesSelfReferencingParticipantSkipsDriverRow
// guards that a self-referencing participant (its Target equals the
// driver's own Target) never returns the driver's own row index.
func TestPlanGeneration_JunctionIndicesSelfReferencingParticipantSkipsDriverRow(t *testing.T) {
	participants := []autoseed.JunctionParticipant{
		{Target: "Person", Fields: []string{"FollowingID"}},
	}
	rowsOf := func(string) int { return 5 }

	for driverRow := 0; driverRow < 5; driverRow++ {
		for blockLocal := 0; blockLocal < 4; blockLocal++ {
			indices := autoseed.JunctionIndices(participants, "Person", driverRow, blockLocal, rowsOf)
			if indices[0] == driverRow {
				t.Fatalf("driverRow %d blockLocal %d: self-referencing participant returned the driver's own row index", driverRow, blockLocal)
			}
		}
	}
}

// TestPlanGeneration_CompositeUniqueConstraintOnForeignKeysDrivesJunctionCap
// guards that junctionCap recognizes a secondary UniqueConstraints entry
// spanning foreign key fields, not only the entity's own primary key —
// the shape EnsureUnique deliberately leaves alone (a reference field's
// row-generation-time value is only a placeholder, not the real parent
// key), so the generation plan must be the one to prevent a duplicate
// (Student, Course) pairing here. selectDriver picks Course (its Target
// sorts first alphabetically), so Student becomes the sole participant.
func TestPlanGeneration_CompositeUniqueConstraintOnForeignKeysDrivesJunctionCap(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Student"},
		{Name: "Course"},
		{
			Name: "Enrollment",
			Fields: []autoseed.Field{
				{Name: "ID", PrimaryKey: true, AutoIncrement: true},
				{Name: "StudentID"},
				{Name: "CourseID"},
			},
			References: []autoseed.Reference{
				{Fields: []string{"StudentID"}, Target: "Student", Nullable: false},
				{Fields: []string{"CourseID"}, Target: "Course", Nullable: false},
			},
			UniqueConstraints: [][]string{{"StudentID", "CourseID"}},
		},
	}

	for seed := uint64(0); seed < 200; seed++ {
		plan := planFor(t, entities, autoseed.NewOptions(autoseed.WithSeed(seed), autoseed.WithScale(3)))
		enrollment := entityPlan(t, plan, "Enrollment")
		if enrollment.Driver != "Course" {
			t.Fatalf("seed %d: Driver = %q, want Course (sorts first alphabetically)", seed, enrollment.Driver)
		}
		student := entityPlan(t, plan, "Student")

		if len(enrollment.JunctionParticipants) != 1 || enrollment.JunctionParticipants[0].Target != "Student" {
			t.Fatalf("seed %d: JunctionParticipants = %v, want exactly one participant targeting Student", seed, enrollment.JunctionParticipants)
		}
		for i, count := range enrollment.ChildCounts {
			if count > student.RowCount {
				t.Fatalf("seed %d: Course row %d wants %d Enrollment children, more than the %d Student rows available to pair with", seed, i, count, student.RowCount)
			}
		}
	}
}

func joinFields(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	out := fields[0]
	for _, f := range fields[1:] {
		out += "+" + f
	}
	return out
}

func TestPlanGeneration_Deterministic(t *testing.T) {
	entities := []autoseed.Entity{
		{Name: "Customer"},
		{Name: "Order", References: []autoseed.Reference{{Fields: []string{"CustomerID"}, Target: "Customer", Nullable: false}}},
	}
	options := autoseed.NewOptions(autoseed.WithSeed(42), autoseed.WithScale(100))

	first := planFor(t, entities, options)
	for i := 0; i < 20; i++ {
		got := planFor(t, entities, options)
		if len(got.Entities) != len(first.Entities) {
			t.Fatalf("run %d: entity count differs", i)
		}
		for j := range got.Entities {
			if got.Entities[j].RowCount != first.Entities[j].RowCount {
				t.Fatalf("run %d: %s.RowCount = %d, want %d", i, got.Entities[j].Entity, got.Entities[j].RowCount, first.Entities[j].RowCount)
			}
		}
	}
}
