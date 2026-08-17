// Command gormseed-basic runs autoseed end to end against a real, on-disk
// SQLite database: migrate three related models, explain the plan autoseed
// derives from them, then seed real rows in dependency order.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type Customer struct {
	ID        uint   `gorm:"primaryKey"`
	Email     string `gorm:"unique"`
	FirstName string
	LastName  string
}

type Order struct {
	ID         uint `gorm:"primaryKey"`
	CustomerID uint `gorm:"not null"`
	Customer   Customer
	CreatedAt  time.Time
}

type OrderItem struct {
	ID       uint `gorm:"primaryKey"`
	OrderID  uint `gorm:"not null"`
	Order    Order
	Price    float64
	Quantity int
	Total    float64
}

func main() {
	const dbPath = "autoseed_example.db"
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		log.Fatalf("removing previous run's %s: %v", dbPath, err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("gorm.Open: %v", err)
	}

	models := []any{&Customer{}, &Order{}, &OrderItem{}}
	if err := db.AutoMigrate(models...); err != nil {
		log.Fatalf("AutoMigrate: %v", err)
	}

	plan, err := gormseed.Explain(db, models)
	if err != nil {
		log.Fatalf("Explain: %v", err)
	}
	fmt.Println(plan.Report())

	ctx := context.Background()
	if err := gormseed.Seed(ctx, db, models, autoseed.WithSeed(42), autoseed.WithScale(200)); err != nil {
		log.Fatalf("Seed: %v", err)
	}

	var customers, orders, items int64
	db.Model(&Customer{}).Count(&customers)
	db.Model(&Order{}).Count(&orders)
	db.Model(&OrderItem{}).Count(&items)
	fmt.Printf("\nSeeded %d customers, %d orders, %d order items into %s\n", customers, orders, items, dbPath)
}
