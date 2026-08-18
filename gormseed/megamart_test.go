package gormseed_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
	"gorm.io/gorm"
)

// The MegaMart fixture is a single model combining every torture case the
// rest of the suite exercises in isolation: a self-reference (Employee), an
// embedded struct (Product.Dimensions), composite primary keys
// (InventoryItem, OrderLine), a shared-primary-key one-to-one
// (ProductProfile), simple unique columns (Company.Name, Warehouse.Code,
// Product.Sku, Order.Reference), soft-delete (Employee, Order), a many2many
// join (Product/Tag) and a correlated derived value (OrderLine.Total).
// Ported in spirit from the .NET sibling's MegaMartSchemaTests.cs, adapted
// to GORM: EF's table-per-type inheritance (Supervisor : Employee,
// DigitalProduct/PhysicalProduct : Product) has no GORM equivalent and is
// left out rather than faked.
type MMCompany struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"unique"`
}

type MMEmployee struct {
	ID        uint `gorm:"primaryKey"`
	CompanyID uint `gorm:"not null"`
	Company   MMCompany
	ManagerID *uint
	Manager   *MMEmployee
	FirstName string
	LastName  string
	HireDate  time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type MMWarehouse struct {
	ID   uint   `gorm:"primaryKey"`
	Code string `gorm:"unique;size:10"`
	Name string
}

type MMDimensions struct {
	Length float64
	Width  float64
	Height float64
}

type MMProduct struct {
	ID          uint   `gorm:"primaryKey"`
	Sku         string `gorm:"unique;size:20"`
	Name        string
	Description string
	Dimensions  MMDimensions `gorm:"embedded"`
	Tags        []*MMTag     `gorm:"many2many:megamart_product_tags;"`
}

type MMTag struct {
	ID       uint         `gorm:"primaryKey"`
	Name     string       `gorm:"unique"`
	Products []*MMProduct `gorm:"many2many:megamart_product_tags;"`
}

type MMProductProfile struct {
	ProductID       uint      `gorm:"primaryKey"`
	Product         MMProduct `gorm:"foreignKey:ProductID"`
	SeoSlug         string    `gorm:"unique;size:60"`
	MetaDescription string
}

type MMInventoryItem struct {
	WarehouseID    uint `gorm:"primaryKey"`
	Warehouse      MMWarehouse
	ProductID      uint `gorm:"primaryKey"`
	Product        MMProduct
	QuantityOnHand int
}

type MMOrder struct {
	ID        uint `gorm:"primaryKey"`
	CompanyID uint `gorm:"not null"`
	Company   MMCompany
	Reference string         `gorm:"unique;size:40"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type MMOrderLine struct {
	OrderID   uint `gorm:"primaryKey"`
	Order     MMOrder
	ProductID uint `gorm:"primaryKey"`
	Product   MMProduct
	UnitPrice float64
	Quantity  int
	Total     float64
}

type MMAuditLogEntry struct {
	ID         uint `gorm:"primaryKey"`
	EntityName string
	ChangedAt  time.Time
}

func megaMartModels() []any {
	return []any{
		&MMCompany{}, &MMEmployee{}, &MMWarehouse{}, &MMProduct{}, &MMTag{}, &MMProductProfile{},
		&MMInventoryItem{}, &MMOrder{}, &MMOrderLine{}, &MMAuditLogEntry{},
	}
}

func TestExplain_MegaMart_CoversEveryEntity(t *testing.T) {
	plan, err := gormseed.Explain(nil, megaMartModels())
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	report := plan.Report()
	for _, name := range []string{
		"MMCompany", "MMEmployee", "MMWarehouse", "MMProduct", "MMTag", "MMProductProfile",
		"MMInventoryItem", "MMOrder", "MMOrderLine", "MMAuditLogEntry",
	} {
		if !strings.Contains(report, name) {
			t.Fatalf("Report() missing entity %s:\n%s", name, report)
		}
	}
}

func TestSeed_Postgres_MegaMart_NoOrphansAcrossEveryReference(t *testing.T) {
	db := postgresDB(t)
	models := megaMartModels()
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(42), autoseed.WithScale(60)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	assertNoOrphans(t, db, "mm_employees", "company_id", "mm_companies", "id")
	assertNoOrphansNullable(t, db, "mm_employees", "manager_id", "mm_employees", "id")
	assertNoOrphans(t, db, "mm_inventory_items", "warehouse_id", "mm_warehouses", "id")
	assertNoOrphans(t, db, "mm_inventory_items", "product_id", "mm_products", "id")
	assertNoOrphans(t, db, "mm_product_profiles", "product_id", "mm_products", "id")
	assertNoOrphans(t, db, "mm_orders", "company_id", "mm_companies", "id")
	assertNoOrphans(t, db, "mm_order_lines", "order_id", "mm_orders", "id")
	assertNoOrphans(t, db, "mm_order_lines", "product_id", "mm_products", "id")

	var orphanJoins int64
	db.Raw(`SELECT count(*) FROM megamart_product_tags j
		LEFT JOIN mm_products p ON j.mm_product_id = p.id
		LEFT JOIN mm_tags t ON j.mm_tag_id = t.id
		WHERE p.id IS NULL OR t.id IS NULL`).Scan(&orphanJoins)
	if orphanJoins != 0 {
		t.Fatalf("%d megamart_product_tags rows reference a non-existent Product or Tag", orphanJoins)
	}
}

func TestSeed_Postgres_MegaMart_SelfReferenceNeverPointsAtItself(t *testing.T) {
	db := postgresDB(t)
	models := megaMartModels()
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(42), autoseed.WithScale(60)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var employees []MMEmployee
	db.Unscoped().Find(&employees)
	for _, e := range employees {
		if e.ManagerID != nil && *e.ManagerID == e.ID {
			t.Fatalf("employee %d is its own manager", e.ID)
		}
	}
}

func TestSeed_Postgres_MegaMart_EmbeddedDimensionsPopulated(t *testing.T) {
	db := postgresDB(t)
	models := megaMartModels()
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(42), autoseed.WithScale(60)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var products []MMProduct
	db.Find(&products)
	if len(products) == 0 {
		t.Fatal("no products were seeded")
	}
	for _, p := range products {
		if p.Dimensions.Length <= 0 || p.Dimensions.Width <= 0 || p.Dimensions.Height <= 0 {
			t.Fatalf("product %d has an unpopulated embedded Dimensions: %+v", p.ID, p.Dimensions)
		}
	}
}

// TestSeed_Postgres_MegaMart_ProductProfileSharesKeyWithoutDuplicating
// guards the shared-primary-key one-to-one shape: MMProductProfile's
// ProductID is both its own primary key and its only foreign key, so at
// most one profile can exist per product.
func TestSeed_Postgres_MegaMart_ProductProfileSharesKeyWithoutDuplicating(t *testing.T) {
	db := postgresDB(t)
	models := megaMartModels()
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(42), autoseed.WithScale(60)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var productCount, profileCount, distinctProductIDs int64
	db.Model(&MMProduct{}).Count(&productCount)
	db.Model(&MMProductProfile{}).Count(&profileCount)
	db.Raw(`SELECT count(DISTINCT product_id) FROM mm_product_profiles`).Scan(&distinctProductIDs)

	if profileCount == 0 {
		t.Fatal("no product profiles were seeded")
	}
	if profileCount > productCount {
		t.Fatalf("ProductProfile count = %d, want at most Product count = %d", profileCount, productCount)
	}
	if profileCount != distinctProductIDs {
		t.Fatalf("%d product profiles but only %d distinct product_id values", profileCount, distinctProductIDs)
	}
}

func TestSeed_Postgres_MegaMart_SoftDeleteBiasesTowardVisible(t *testing.T) {
	db := postgresDB(t)
	models := megaMartModels()
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(42), autoseed.WithScale(200)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	assertMostlyVisible(t, db, "mm_employees")
	assertMostlyVisible(t, db, "mm_orders")
}

func assertMostlyVisible(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	var total, deleted int64
	db.Table(table).Count(&total)
	db.Table(table).Unscoped().Where("deleted_at IS NOT NULL").Count(&deleted)

	if total == 0 {
		t.Fatalf("%s has no rows", table)
	}
	if deleted == 0 {
		t.Fatalf("%s: no deleted rows out of %d, want at least one", table, total)
	}
	if visible := total - deleted; visible <= deleted {
		t.Fatalf("%s: visible rows (%d) are not the majority over deleted rows (%d)", table, visible, deleted)
	}
}

func TestSeed_Postgres_MegaMart_OrderLineTotalMatchesUnitPriceTimesQuantity(t *testing.T) {
	db := postgresDB(t)
	models := megaMartModels()
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(42), autoseed.WithScale(60)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var lines []MMOrderLine
	db.Find(&lines)
	if len(lines) == 0 {
		t.Fatal("no order lines were seeded")
	}
	for _, line := range lines {
		want := float64(int(line.UnitPrice*float64(line.Quantity)*100+0.0)) / 100
		if diff := line.Total - want; diff > 0.011 || diff < -0.011 {
			t.Fatalf("order line %d/%d: Total = %v, want approximately UnitPrice*Quantity = %v", line.OrderID, line.ProductID, line.Total, want)
		}
	}
}

func TestSeed_Postgres_MegaMart_Deterministic(t *testing.T) {
	db1 := postgresDB(t)
	db2 := postgresDB(t)
	models := megaMartModels()

	rowCounts := func(db *gorm.DB) map[string]int64 {
		counts := make(map[string]int64, len(models))
		for _, table := range []string{"mm_companies", "mm_employees", "mm_warehouses", "mm_products", "mm_tags", "mm_product_profiles", "mm_inventory_items", "mm_orders", "mm_order_lines", "mm_audit_log_entries"} {
			var count int64
			db.Table(table).Count(&count)
			counts[table] = count
		}
		return counts
	}

	for _, db := range []*gorm.DB{db1, db2} {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("AutoMigrate: %v", err)
		}
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(7), autoseed.WithScale(40)); err != nil {
			t.Fatalf("Seed: %v", err)
		}
	}

	counts1, counts2 := rowCounts(db1), rowCounts(db2)
	for table, c1 := range counts1 {
		if c2 := counts2[table]; c1 != c2 {
			t.Fatalf("%s row count differs across runs with the same seed: %d vs %d", table, c1, c2)
		}
	}
}
