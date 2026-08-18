# autoseed

Seed your database from your ORM model. One call, full referential integrity, realistic distribution. No factories, no YAML files, no ordering files by number to get foreign keys right.

From the author of [EFCore.AutoSeed](https://github.com/danellalc/EFCore.AutoSeed) — the same idea, native to Go.

> [GIF placeholder — will be recorded from the first real run before launch.]

## Status

In development. This README describes the full design being built — see the [roadmap](ARCHITECTURE.md#roadmap) for what ships when.

**Works today:** reading a GORM model, `gormseed.Explain`, and `gormseed.Seed` — real inserts, in dependency order, with long-tail cardinality, unique-field dedup, and deferred second-pass cycles, tested against real PostgreSQL and MySQL.

**Not built yet:** `gormseed.SeedCoverage`, `WithLocale`, `WithNilRate`, and every value-realism knob beyond the ~16 built-in inference rules (dirty data, weekday/business-hour clustering, per-column null rate). Code blocks below that use them are the design, marked as such inline.

## The problem

Every Go seeding tool available today asks **you** to describe the model again — in YAML fixtures, in seeder structs with hand-written loops, in JSON files named `01_role.json`, `02_user.json` so the insertion order works out. You already declared all of it in your GORM tags or your ent schema. Then the model changes and your seeder breaks.

The best value generator in the ecosystem says it plainly: gofakeit generates individual data points — interrelated data sets with consistent foreign keys require additional manual logic outside the library. That manual logic is what everybody keeps rewriting. This is it, written once.

And the data you end up with is uniform: every customer with three orders. In production one customer has 500,000. The query planner picks a different plan for each shape, so **your performance test passes while lying to you**.

## Usage

```go
import (
    "github.com/danellalc/autoseed"
    "github.com/danellalc/autoseed/gormseed"
)

err := gormseed.Seed(ctx, db, []any{&Customer{}, &Order{}, &OrderItem{}},
    autoseed.WithSeed(42),
    autoseed.WithScale(1_000),
)
```

That is the whole API for the common case, and it runs today. GORM keeps no registry of every struct you have used — unlike an EF Core `DbContext`, a `*gorm.DB` cannot tell you what it knows — so the model list is the one thing you state; autoseed works out the insertion order, resolves cycles, infers what each field means, and writes referentially valid rows, one entity type at a time, foreign keys copied from the real, already-inserted parent row.

A full, runnable version of this example — real SQLite database, `Explain` and `Seed` both — lives in [`examples/gormseed-basic`](examples/gormseed-basic); `go run .` there does the whole thing with no setup.

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

Built on gofakeit for values (it does not replace it), autoseed adds the shape. Related row counts are drawn per parent from an exponential long tail, not a flat average:

```
Customer:   1,000 rows
Order:      3,847 rows   long tail: mean 3.8, max 512, one customer holds 13%
OrderItem: 19,203 rows
```

Most customers have one order. A few have hundreds — sometimes zero. Coherent values agree with each other on the same row (`FirstName`+`LastName`+`Email`, `Price`×`Quantity`=`Total`, `UpdatedAt` at or after `CreatedAt`). Weekday/business-hour clustering and dirty data are not built yet — see the [roadmap](ARCHITECTURE.md#roadmap).

### It explains itself before it writes anything

Works today — the one example on this page that actually runs:

```go
plan, err := gormseed.Explain(db, []any{&Customer{}, &Order{}, &OrderItem{}})
fmt.Println(plan.Report())
```

Prints the insertion order, which cycles got deferred to a second pass, and which constructs were skipped and why. Nothing is written. `GenerationPlan` itself already computes row counts for `Seed` internally; wiring them into `Explain`'s report too is still open — see the [roadmap](ARCHITECTURE.md#roadmap).

### Coverage mode

The opposite of bulk. The *smallest* dataset that exercises everything:

```go
// Design target — not implemented yet, see Status above. Use gormseed.Seed
// with a small WithScale for something that runs today.
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

A property-based test asserts that for **any** model and **any** mix of nullable/required references, the engine either names an unsatisfiable cycle or produces an order that respects every foreign key; it runs on every commit against the graph and cycle engine directly, no database needed. A second layer of property tests runs `gormseed.Seed` itself, seed and scale rapid-varied, against a real containerized PostgreSQL and checks every row for orphaned foreign keys and primary key collisions — a linear chain, a diamond of two required principals merging into one dependent, a nullable self-reference, a composite-primary-key junction, a shared-primary-key one-to-one, and a self-referencing many-to-many (a "follows" table, where two foreign keys target the same entity and only their own columns tell them apart).

`gormseed.Seed` is tested against real, containerized PostgreSQL — never SQLite-only — covering a required-FK chain with long-tail cardinality, a nullable self-reference, a required/nullable two-entity cycle resolved in a second pass, and a many-to-many join table. A combined "MegaMart" model exercises every one of those shapes together in a single seed run — self-reference, embedded struct, composite and shared primary keys, unique columns, soft-delete bias, a many-to-many join and a correlated derived value — the way a real application model mixes them. MySQL gets its own real-container test for the required-FK path and its `LastInsertId` batch arithmetic, not yet the same depth. Against real, public schemas is still ahead of launch.

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
