package gormseed_test

import (
	"context"
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func postgresDB(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping testcontainers integration test in -short mode")
	}

	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("autoseed"),
		tcpostgres.WithUsername("autoseed"),
		tcpostgres.WithPassword("autoseed"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminating postgres container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// Foreign key constraints off: a genuine reference cycle (Fase 3's own
	// deferred-edge case) can never satisfy both sides' inline constraint
	// at CREATE TABLE time regardless of migration order. Referential
	// correctness is verified in Go, against the real inserted rows,
	// which is a stronger check than a constraint would give anyway.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return db
}

type PGCustomer struct {
	ID    uint   `gorm:"primaryKey"`
	Email string `gorm:"unique"`
}

type PGOrder struct {
	ID         uint `gorm:"primaryKey"`
	CustomerID uint `gorm:"not null"`
	Customer   PGCustomer
	Street     string
	City       string
	CreatedAt  time.Time
}

type PGOrderItem struct {
	ID       uint `gorm:"primaryKey"`
	OrderID  uint `gorm:"not null"`
	Order    PGOrder
	Price    float64
	Quantity int
	Total    float64
}

func TestSeed_Postgres_BasicChain(t *testing.T) {
	db := postgresDB(t)
	models := []any{&PGCustomer{}, &PGOrder{}, &PGOrderItem{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(42), autoseed.WithScale(50))
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var customerCount, orderCount, itemCount int64
	db.Model(&PGCustomer{}).Count(&customerCount)
	db.Model(&PGOrder{}).Count(&orderCount)
	db.Model(&PGOrderItem{}).Count(&itemCount)

	if customerCount != 50 {
		t.Fatalf("Customer count = %d, want 50", customerCount)
	}
	if orderCount == 0 {
		t.Fatal("Order count = 0, want at least some orders")
	}

	var orphanOrders int64
	db.Raw(`SELECT count(*) FROM pg_orders o LEFT JOIN pg_customers c ON o.customer_id = c.id WHERE c.id IS NULL`).Scan(&orphanOrders)
	if orphanOrders != 0 {
		t.Fatalf("%d orders reference a non-existent customer", orphanOrders)
	}

	var distinctCustomersWithOrders int64
	db.Raw(`SELECT count(DISTINCT customer_id) FROM pg_orders`).Scan(&distinctCustomersWithOrders)
	if orderCount == distinctCustomersWithOrders {
		t.Fatalf("every order has a distinct customer_id (%d orders, %d distinct) — want the long-tail driver to give at least one customer more than one order", orderCount, distinctCustomersWithOrders)
	}

	var orphanItems int64
	db.Raw(`SELECT count(*) FROM pg_order_items i LEFT JOIN pg_orders o ON i.order_id = o.id WHERE o.id IS NULL`).Scan(&orphanItems)
	if orphanItems != 0 {
		t.Fatalf("%d order items reference a non-existent order", orphanItems)
	}

	var emptyStreet int64
	db.Model(&PGOrder{}).Where("street = ''").Count(&emptyStreet)
	if emptyStreet > 0 {
		t.Fatalf("%d orders have an empty embedded Street, want it populated", emptyStreet)
	}

	var duplicateEmails int64
	db.Raw(`SELECT count(*) FROM (SELECT email FROM pg_customers GROUP BY email HAVING count(*) > 1) d`).Scan(&duplicateEmails)
	if duplicateEmails != 0 {
		t.Fatalf("%d duplicate customer emails, want unique enforced", duplicateEmails)
	}

	var items []PGOrderItem
	db.Find(&items)
	for _, item := range items {
		want := float64(int(item.Price*float64(item.Quantity)*100+0.0)) / 100
		if diff := item.Total - want; diff > 0.011 || diff < -0.011 {
			t.Fatalf("OrderItem %d: Total = %v, want approximately Price*Quantity = %v", item.ID, item.Total, want)
		}
	}
}

type PGEmployee struct {
	ID        uint `gorm:"primaryKey"`
	ManagerID *uint
	Manager   *PGEmployee
}

func TestSeed_Postgres_NullableSelfReference(t *testing.T) {
	db := postgresDB(t)
	models := []any{&PGEmployee{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(1), autoseed.WithScale(20)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var employees []PGEmployee
	db.Find(&employees)
	if len(employees) != 20 {
		t.Fatalf("Employee count = %d, want 20", len(employees))
	}

	ids := make(map[uint]bool, len(employees))
	for _, e := range employees {
		ids[e.ID] = true
	}
	for _, e := range employees {
		if e.ManagerID == nil {
			continue
		}
		if *e.ManagerID == e.ID {
			t.Fatalf("employee %d is its own manager", e.ID)
		}
		if !ids[*e.ManagerID] {
			t.Fatalf("employee %d has ManagerID %d, which does not exist", e.ID, *e.ManagerID)
		}
	}
}

type PGOrderCycle struct {
	ID        uint      `gorm:"primaryKey"`
	ContactID uint      `gorm:"not null"`
	Contact   PGContact `gorm:"foreignKey:ContactID"`
}

type PGContact struct {
	ID             uint `gorm:"primaryKey"`
	DefaultOrderID *uint
	DefaultOrder   *PGOrderCycle `gorm:"foreignKey:DefaultOrderID"`
}

func TestSeed_Postgres_DeferredCycle(t *testing.T) {
	db := postgresDB(t)
	models := []any{&PGOrderCycle{}, &PGContact{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(7), autoseed.WithScale(15)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var orderIDs []uint
	db.Model(&PGOrderCycle{}).Pluck("id", &orderIDs)
	validOrder := make(map[uint]bool, len(orderIDs))
	for _, id := range orderIDs {
		validOrder[id] = true
	}

	var contacts []PGContact
	db.Find(&contacts)
	for _, c := range contacts {
		if c.DefaultOrderID != nil && !validOrder[*c.DefaultOrderID] {
			t.Fatalf("contact %d.DefaultOrderID = %d does not reference an existing order", c.ID, *c.DefaultOrderID)
		}
	}
}

type PGPost struct {
	ID   uint     `gorm:"primaryKey"`
	Tags []*PGTag `gorm:"many2many:pg_post_tags;"`
}

type PGTag struct {
	ID    uint      `gorm:"primaryKey"`
	Posts []*PGPost `gorm:"many2many:pg_post_tags;"`
}

func TestSeed_Postgres_ManyToMany(t *testing.T) {
	db := postgresDB(t)
	models := []any{&PGPost{}, &PGTag{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(3), autoseed.WithScale(10)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var joinCount int64
	db.Table("pg_post_tags").Count(&joinCount)
	if joinCount == 0 {
		t.Fatal("pg_post_tags is empty, want at least some join rows")
	}

	var orphanJoins int64
	db.Raw(`SELECT count(*) FROM pg_post_tags j
		LEFT JOIN pg_posts p ON j.pg_post_id = p.id
		LEFT JOIN pg_tags t ON j.pg_tag_id = t.id
		WHERE p.id IS NULL OR t.id IS NULL`).Scan(&orphanJoins)
	if orphanJoins != 0 {
		t.Fatalf("%d join rows reference a non-existent Post or Tag", orphanJoins)
	}
}

func TestSeed_Postgres_Deterministic(t *testing.T) {
	db1 := postgresDB(t)
	db2 := postgresDB(t)
	models := []any{&PGCustomer{}, &PGOrder{}, &PGOrderItem{}}

	for _, db := range []*gorm.DB{db1, db2} {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("AutoMigrate: %v", err)
		}
		if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(99), autoseed.WithScale(30)); err != nil {
			t.Fatalf("Seed: %v", err)
		}
	}

	var count1, count2 int64
	db1.Model(&PGOrder{}).Count(&count1)
	db2.Model(&PGOrder{}).Count(&count2)
	if count1 != count2 {
		t.Fatalf("Order count differs across runs with the same seed: %d vs %d", count1, count2)
	}

	var orders1, orders2 []PGOrder
	db1.Order("id").Find(&orders1)
	db2.Order("id").Find(&orders2)
	for i := range orders1 {
		if orders1[i].Street != orders2[i].Street || orders1[i].City != orders2[i].City {
			t.Fatalf("row %d differs across runs: %+v vs %+v", i, orders1[i], orders2[i])
		}
	}
}
