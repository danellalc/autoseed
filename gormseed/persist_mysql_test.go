package gormseed_test

import (
	"context"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func mysqlDB(t *testing.T) *gorm.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping testcontainers integration test in -short mode")
	}

	ctx := context.Background()
	container, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("autoseed"),
		tcmysql.WithUsername("autoseed"),
		tcmysql.WithPassword("autoseed"),
	)
	if err != nil {
		t.Fatalf("starting mysql container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("terminating mysql container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return db
}

type MyCustomer struct {
	ID    uint   `gorm:"primaryKey"`
	Email string `gorm:"unique;size:191"`
}

type MyOrder struct {
	ID         uint `gorm:"primaryKey"`
	CustomerID uint `gorm:"not null"`
	Customer   MyCustomer
}

type MyOrderItem struct {
	ID       uint `gorm:"primaryKey"`
	OrderID  uint `gorm:"not null"`
	Order    MyOrder
	Price    float64
	Quantity int
}

// TestSeed_MySQL_BatchIDReadback is the sharp edge PLANO.md calls out by
// name: MySQL's LastInsertId only returns the FIRST id of a multi-row
// INSERT, so GORM's own driver must compute the rest by incrementing from
// it. This only proves out against a real server with real
// auto_increment, never SQLite.
func TestSeed_MySQL_BatchIDReadback(t *testing.T) {
	db := mysqlDB(t)
	models := []any{&MyCustomer{}, &MyOrder{}, &MyOrderItem{}}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(11), autoseed.WithScale(80)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	var customerCount, orderCount, itemCount int64
	db.Model(&MyCustomer{}).Count(&customerCount)
	db.Model(&MyOrder{}).Count(&orderCount)
	db.Model(&MyOrderItem{}).Count(&itemCount)
	if customerCount != 80 {
		t.Fatalf("Customer count = %d, want 80", customerCount)
	}
	if orderCount == 0 || itemCount == 0 {
		t.Fatalf("Order count = %d, OrderItem count = %d, want both non-zero", orderCount, itemCount)
	}

	var customerIDs []uint
	db.Model(&MyCustomer{}).Pluck("id", &customerIDs)
	seen := make(map[uint]bool, len(customerIDs))
	for _, id := range customerIDs {
		if id == 0 {
			t.Fatal("a customer has ID 0: batch insert failed to read back its generated id")
		}
		if seen[id] {
			t.Fatalf("duplicate customer ID %d: batch insert ID readback is wrong", id)
		}
		seen[id] = true
	}

	var orphanOrders int64
	db.Raw(`SELECT count(*) FROM my_orders o LEFT JOIN my_customers c ON o.customer_id = c.id WHERE c.id IS NULL`).Scan(&orphanOrders)
	if orphanOrders != 0 {
		t.Fatalf("%d orders reference a non-existent customer — MySQL LastInsertId batch arithmetic produced a wrong id", orphanOrders)
	}

	var orphanItems int64
	db.Raw(`SELECT count(*) FROM my_order_items i LEFT JOIN my_orders o ON i.order_id = o.id WHERE o.id IS NULL`).Scan(&orphanItems)
	if orphanItems != 0 {
		t.Fatalf("%d order items reference a non-existent order", orphanItems)
	}

	var duplicateEmails int64
	db.Raw(`SELECT count(*) FROM (SELECT email FROM my_customers GROUP BY email HAVING count(*) > 1) d`).Scan(&duplicateEmails)
	if duplicateEmails != 0 {
		t.Fatalf("%d duplicate customer emails, want unique enforced", duplicateEmails)
	}
}
