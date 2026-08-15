# autoseed

Seed your database from your ORM model. One call, full referential integrity, realistic distribution. No factories, no YAML files, no ordering files by number to get foreign keys right.

From the author of [EFCore.AutoSeed](https://github.com/danellalc/EFCore.AutoSeed) — the same idea, native to Go.

> Status: in development. This README describes the design being built — see the [roadmap](ARCHITECTURE.md#roadmap).

> [GIF placeholder — will be recorded from the first real run before launch.]

## The problem

Every Go seeding tool available today asks **you** to describe the model again — in YAML fixtures, in seeder structs with hand-written loops, in JSON files named `01_role.json`, `02_user.json` so the insertion order works out. You already declared all of it in your GORM tags or your ent schema. Then the model changes and your seeder breaks.

The best value generator in the ecosystem says it plainly: gofakeit generates individual data points — interrelated data sets with consistent foreign keys require additional manual logic outside the library. That manual logic is what everybody keeps rewriting. This is it, written once.

And the data you end up with is uniform: every customer with three orders. In production one customer has 500,000. The query planner picks a different plan for each shape, so **your performance test passes while lying to you**.

## Usage

```go
import "github.com/danellalc/autoseed/gormseed"

err := gormseed.Seed(db, []any{&Customer{}, &Order{}, &OrderItem{}},
    autoseed.WithSeed(42),
    autoseed.WithScale(1_000),
)
```

That is the whole API for the common case. GORM keeps no registry of every struct you have used — unlike an EF Core `DbContext`, a `*gorm.DB` cannot tell you what it knows — so the model list is the one thing you state; autoseed works out the insertion order, resolves cycles, infers what each field means, and writes referentially valid rows.

Same seed, same data. Always.

```bash
go get github.com/danellalc/autoseed
```

## What makes it different

### It reads the ORM model, not the database schema

DDL introspection sees tables and columns. The ORM model also carries associations, embedded structs, polymorphic relations and soft-delete conventions.

autoseed generates data your **database accepts and your application can actually read** — soft-deleted rows in realistic proportion, embedded structs populated instead of left zero-valued, association tables filled on both sides.

It also works before the database exists.

### It works out the order itself

Foreign keys form a graph. autoseed topologically sorts it (stable: ties break on entity name), detects cycles, and resolves the nullable ones with a second pass.

No numbered files. No ordering by hand.

When a cycle is genuinely unsatisfiable — a required foreign key with no nullable link — autoseed names the entities involved and returns an error, instead of letting your database throw a constraint violation.

### It generates realistic distributions, not just realistic values

Built on gofakeit for values (it does not replace it), autoseed adds the shape:

```
Customer:   1,000 rows
Order:      3,847 rows   long tail: mean 3.8, max 512, one customer holds 13%
OrderItem: 19,203 rows
```

Most customers have one order. A few have hundreds. Timestamps cluster on weekdays and business hours. Optional fields are actually nil sometimes.

### It explains itself before it writes anything

```go
plan, err := gormseed.Explain(db, []any{&Customer{}, &Order{}, &OrderItem{}}, autoseed.WithSeed(42), autoseed.WithScale(1_000))
fmt.Println(plan.Report())
```

Prints the insertion order, row counts per entity, which cycles got deferred to a second pass, and which entities were skipped and why. Nothing is written.

### Coverage mode

The opposite of bulk. The *smallest* dataset that exercises everything:

```go
err := gormseed.SeedCoverage(db, []any{&Customer{}, &Order{}, &OrderItem{}})
```

Every enum-like field value, every nullable field in both states, every relationship at zero, one and many, every string at empty, one char and max length. Usually under 50 rows.

## Adapters

The core is ORM-agnostic. Each ORM gets an adapter that reads its model:

| Adapter | Reads | Status |
|---|---|---|
| `autoseed/gormseed` | GORM struct tags via `schema.Parse` | **v1** |
| `autoseed/entseed` | ent's generated graph (`gen.Graph`) | v2 |
| sqlc, Bun | — | roadmap, on demand |

GORM first because it is where most Go codebases are. ent second because its schema **is** a graph — nodes and edges are exposed by codegen, which makes it the technically sweetest target.

## What it does not do

- **It does not anonymise production data.** Not a masking tool.
- **It is not a service.** No cloud, no account, no server.
- **GORM and ent only** (for now). Not raw `database/sql`, not every ORM. Adapters are on demand, with traction, never speculatively — two tools in this category died of scope creep.
- **PostgreSQL, MySQL and SQLite only.**
- **It refuses models it cannot satisfy**, loudly and by name.

## Validated

Property-based tests assert that for **any** model and **any** seed, every foreign key points at an existing row and no constraint is violated. They run on every commit, against real databases via testcontainers-go.

Also tested against real, public schemas (list to come at launch).

## Compared to

| Tool | Approach |
|---|---|
| **gofakeit** | generates values for structs; autoseed is built on it and does not replace it |
| **go-faker/faker** | fills structs by reflection, no relationships |
| **gorm-seeder / gormseeder** | you write the `Seed()` loops yourself |
| **populator, testfixtures** | YAML fixtures you write; ordering is yours to manage |
| **fabricator** | generics factories you define per type |
| **EF Core / .NET** | [EFCore.AutoSeed](https://github.com/danellalc/EFCore.AutoSeed), same author, same design |

They are all either value generators or manual seeders. None reads the model and derives the order. That is the gap this fills.

## License

MIT
