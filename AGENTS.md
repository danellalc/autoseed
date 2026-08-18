# AGENTS.md

Guidance for AI coding agents working in this repository.

If you are consuming this library rather than developing it, read [llms.txt](llms.txt) instead.

## What this project is

A library that seeds a database by reading the ORM model. The model is the specification — nothing about entities, ordering or relationships is declared by hand. Sibling of EFCore.AutoSeed (.NET, same author, same pipeline); when a design question arises, check how the sibling solved it first.

## Build and test

```bash
go build ./...
go test ./...              # everything (needs Docker for integration)
go test ./... -short       # no Docker
go test -run TestCycle ./...
go vet ./... && golangci-lint run
```

Integration tests use testcontainers-go against real PostgreSQL and MySQL. SQLite is a supported target but NEVER the only test surface — it accepts data the real engines reject.

## The pipeline

Seven stages; new code belongs to exactly one:

1. `ModelReader` (in adapters) — entities, fields, keys, references, soft-delete, embedded
2. `DependencyGraph` — stable topological sort, ties break on entity name
3. `CycleResolver` — nullable cycles → two passes; required cycles → `ErrUnsatisfiableCycle` naming entities
4. `GenerationPlan` — row counts, cardinality, distribution
5. `ValueGeneration` — semantic inference over gofakeit, seeded
6. `ConstraintSatisfaction` — uniqueness (generate, dedupe with a numeric suffix, bounded retries), not-null, length
7. `Persistence` — ordered batched insert, generated IDs read back before children

## Hard rules

**Determinism is the central guarantee.** Same seed, same data, always.
- All randomness derives from the root seed, hierarchically and positionally: root → entity → row index → field.
- **Never iterate a map where order affects output** — Go randomizes map order by design; sort keys first. This is the #1 trap in this codebase and has a dedicated test.
- No global `math/rand`, no `time.Now()` in the generation path.
- Generation is sequential; insertion may batch. Changing generated output for a given seed is a breaking change (major — and in Go, a new major means a new import path, so avoid).

**The core knows no ORM, and no value generator.** Root package depends on stdlib only. gofakeit lives in `/inference`, never the root — the same reason the .NET sibling keeps Bogus out of `AutoSeed.Core`: the graph and cycle engine stay testable without a value generator. `import "gorm.io/gorm"` or `import ".../gofakeit"` outside their own package means the modeling is wrong — stop and refactor. Adapters implement `ModelSource`; everything downstream consumes only that contract.

**Never re-derive GORM conventions by hand.** Use `schema.Parse` and `schema.Relationships`. GORM resolves; we read.

**Errors, never panics.** Typed sentinel errors (`ErrUnsatisfiableCycle`, `ErrUnsupportedField`, `ErrNilSeed`, `ErrInvalidScale`, `gormseed.ErrNilDB`) wrapped with `%w`, friendly to `errors.Is`/`errors.As`, naming the entity and field in English. Validate at the boundary: a nil `*SeededSource`/`*gorm.DB` or a negative `Scale` gets caught by the exported entry point that receives it, before it reaches a nil dereference or a negative-length `make`.

**Fail by name, never silently.** Unsupported constructs are skipped BY NAME in the Explain report or returned as typed errors. Silently mis-generated data is the worst failure mode this project exists to prevent.

**Embedded structs are columns, not entities.** Soft-delete rows are generated mostly-visible.

## Code style

- No comments on unexported identifiers — descriptive names instead. No emojis.
- **Godoc on every exported identifier is mandatory** (starts with the name). It ships to pkg.go.dev and is the project's storefront.
- `gofmt`, `go vet`, `golangci-lint` clean. Short lowercase package names (`gormseed`).
- `context.Context` first parameter on anything touching a database, and actually threaded into the query/exec call (`db.WithContext(ctx)`) — accepting it without wiring it through is a real bug, not a style nit; a canceled context must stop in-flight writes. Error as last return.
- Accept interfaces, return structs. Functional options for configuration.
- Table-driven tests as the default.

## Testing expectations

- Property-based (pgregory.net/rapid): for any model and any seed, every FK references an existing row.
- Determinism: same seed twice → byte-identical output; anti-map-iteration test runs repeatedly.
- New inference rules and distribution shapes ship with tests.
- The MegaMart torture model (self-reference, embedded struct, composite and shared primary keys, unique columns, soft-delete, many2many, correlated derived values, all in one seed run) must stay green. Composite unique constraints and a composite primary key of three or more foreign keys are out of scope — not modeled.
- A composite-primary-key junction entity or a shared-primary-key one-to-one must never generate more children for a driver row than the primary key can hold without colliding — cap `GenerationPlan`'s draw, don't rely on the round-robin assignment in `persist.go` to paper over it. Match a reference by its own `Fields`, never by `Target` alone: two references can share a target (a self-referencing many-to-many), and only their own foreign key columns tell them apart. Any new "child's key is derived from the parent's" shape gets a property test at small scale (1-4), where the collision is easiest to trigger — and if the shape is self-referencing, assert no row pairs with itself, not just that no primary key repeats.
- A primary key field is skipped by `GenerateRow` only when it's auto-increment; any other primary key field (a natural key, or the one non-FK column of a mixed composite key) must be generated like any other field — never assume "PrimaryKey" alone means "someone else fills this in."

## Commits

Conventional commits in English. Scopes: `core`, `gormseed`, `entseed`, `graph`, `inference`, `cli`.

```
feat(graph): stable topological sort with name tiebreak
fix(gormseed): read composite primary keys from schema.Parse
```

Never mention AI assistance, Claude, or co-authorship in commit messages.

## Out of scope

Do not implement, and close issues requesting: production data anonymisation, cloud/hosted anything, ORMs beyond GORM and ent without demonstrated traction, databases beyond PostgreSQL/MySQL/SQLite, speculative adapters.

Two tools in this category died of scope creep. The boundary is the survival strategy.
