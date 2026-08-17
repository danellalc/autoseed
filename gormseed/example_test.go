package gormseed_test

import (
	"context"
	"fmt"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/gormseed"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type ExampleCustomer struct {
	ID    uint   `gorm:"primaryKey"`
	Email string `gorm:"unique"`
}

type ExampleOrder struct {
	ID         uint `gorm:"primaryKey"`
	CustomerID uint `gorm:"not null"`
	Customer   ExampleCustomer
}

func ExampleSeed() {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxOpenConns(1)

	models := []any{&ExampleCustomer{}, &ExampleOrder{}}
	if err := db.AutoMigrate(models...); err != nil {
		panic(err)
	}

	if err := gormseed.Seed(context.Background(), db, models, autoseed.WithSeed(1), autoseed.WithScale(10)); err != nil {
		panic(err)
	}

	var customerCount int64
	db.Model(&ExampleCustomer{}).Count(&customerCount)
	fmt.Println(customerCount)
	// Output: 10
}

func ExampleExplain() {
	models := []any{&ExampleCustomer{}, &ExampleOrder{}}

	plan, err := gormseed.Explain(nil, models)
	if err != nil {
		panic(err)
	}

	fmt.Print(plan.Report())
	// Output:
	// Insertion order:
	//   1. ExampleCustomer
	//   2. ExampleOrder
}
