package entseed_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/entseed"
	"github.com/danellalc/autoseed/entseed/internal/entfixtures"
)

const schemaPath = "./internal/entfixtures/schema"

func entClient(t *testing.T) *entfixtures.Client {
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

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("closing sql.DB: %v", err)
		}
	})

	client := entfixtures.NewClient(entfixtures.Driver(entsql.OpenDB("postgres", sqlDB)))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("closing ent client: %v", err)
		}
	})

	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("creating schema: %v", err)
	}
	return client
}

func TestSeed_Postgres_BasicChain(t *testing.T) {
	client := entClient(t)
	ctx := context.Background()

	if err := entseed.Seed(ctx, client, schemaPath, autoseed.WithSeed(42), autoseed.WithScale(50)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	customerCount, err := client.Customer.Query().Count(ctx)
	if err != nil {
		t.Fatalf("counting customers: %v", err)
	}
	if customerCount != 50 {
		t.Fatalf("Customer count = %d, want 50", customerCount)
	}

	orderCount, err := client.Order.Query().Count(ctx)
	if err != nil {
		t.Fatalf("counting orders: %v", err)
	}
	if orderCount == 0 {
		t.Fatal("Order count = 0, want at least some orders")
	}

	orders, err := client.Order.Query().All(ctx)
	if err != nil {
		t.Fatalf("querying orders: %v", err)
	}
	for _, o := range orders {
		customer, err := o.QueryCustomer().Only(ctx)
		if err != nil {
			t.Fatalf("order %d has no valid customer: %v", o.ID, err)
		}
		if customer.Email == "" {
			t.Fatalf("order %d's customer has an empty email", o.ID)
		}
	}

	emails := make(map[string]bool, customerCount)
	customers, err := client.Customer.Query().All(ctx)
	if err != nil {
		t.Fatalf("querying customers: %v", err)
	}
	for _, c := range customers {
		if emails[c.Email] {
			t.Fatalf("duplicate customer email %q, want unique enforced", c.Email)
		}
		emails[c.Email] = true
	}
}

func TestSeed_Postgres_NullableSelfReference(t *testing.T) {
	client := entClient(t)
	ctx := context.Background()

	if err := entseed.Seed(ctx, client, schemaPath, autoseed.WithSeed(1), autoseed.WithScale(20)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	employees, err := client.Employee.Query().All(ctx)
	if err != nil {
		t.Fatalf("querying employees: %v", err)
	}
	if len(employees) != 20 {
		t.Fatalf("Employee count = %d, want 20", len(employees))
	}

	for _, e := range employees {
		manager, err := e.QueryManager().Only(ctx)
		if err != nil {
			if entfixtures.IsNotFound(err) {
				continue
			}
			t.Fatalf("employee %d: querying manager: %v", e.ID, err)
		}
		if manager.ID == e.ID {
			t.Fatalf("employee %d is its own manager", e.ID)
		}
	}
}

func TestExplain_Basic(t *testing.T) {
	plan, err := entseed.Explain(schemaPath)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	report := plan.Report()
	for _, name := range []string{"Customer", "Order", "Employee"} {
		want := name + "\n"
		if !containsLine(report, want) {
			t.Fatalf("Report() missing entity %s:\n%s", name, report)
		}
	}

	customerPos := indexOf(report, "Customer\n")
	orderPos := indexOf(report, "Order\n")
	if customerPos == -1 || orderPos == -1 || customerPos > orderPos {
		t.Fatalf("Customer must be inserted before Order, got:\n%s", report)
	}
}

// TestExplain_ManyToManySkippedNamed guards the "fail by name, never
// silently" rule: a many-to-many edge (Product.tags/Tag.products) is a
// real, common ent shape entseed doesn't generate for yet. Explain must
// name it in the Skipped section, not just drop it with no trace, and
// name it exactly once even though ent declares it from both sides.
func TestExplain_ManyToManySkippedNamed(t *testing.T) {
	plan, err := entseed.Explain(schemaPath)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	report := plan.Report()
	if !containsLine(report, "Skipped:\n") {
		t.Fatalf("Report() missing a Skipped section for the Product/Tag many-to-many:\n%s", report)
	}
	if count := strings.Count(report, "many-to-many edges are read but not generated yet"); count != 1 {
		t.Fatalf("many-to-many skip reason appears %d times, want exactly 1 (reported from one side only):\n%s", count, report)
	}
}

// TestSeed_Postgres_ManyToManySkippedDoesNotBlockOtherEntities guards
// that a skipped many-to-many edge doesn't stop Product and Tag from
// being seeded on their own -- only the association between them is
// left ungenerated.
func TestSeed_Postgres_ManyToManySkippedDoesNotBlockOtherEntities(t *testing.T) {
	client := entClient(t)
	ctx := context.Background()

	if err := entseed.Seed(ctx, client, schemaPath, autoseed.WithSeed(3), autoseed.WithScale(10)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	productCount, err := client.Product.Query().Count(ctx)
	if err != nil {
		t.Fatalf("counting products: %v", err)
	}
	if productCount != 10 {
		t.Fatalf("Product count = %d, want 10", productCount)
	}

	tagCount, err := client.Tag.Query().Count(ctx)
	if err != nil {
		t.Fatalf("counting tags: %v", err)
	}
	if tagCount != 10 {
		t.Fatalf("Tag count = %d, want 10", tagCount)
	}
}

func containsLine(s, substr string) bool {
	return indexOf(s, substr) != -1
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
