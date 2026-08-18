package autoseed_test

import (
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
	return autoseed.PlanGeneration(entities, result.Order, result.Deferred, autoseed.NewSeededSource(options.Seed), options)
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

		if item.JunctionTarget != "Warehouse" {
			t.Fatalf("seed %d: JunctionTarget = %q, want Warehouse", seed, item.JunctionTarget)
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
	if item.JunctionTarget != "" {
		t.Fatalf("JunctionTarget = %q, want empty: OrderItem's primary key is its own surrogate ID, not the two references", item.JunctionTarget)
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
