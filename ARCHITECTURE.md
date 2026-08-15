# Architecture

## Packaging

One module, adapters as subpackages. **No separate repos, no separate modules** — one `go get` brings everything, and the adapter you don't import costs you nothing.

```
github.com/danellalc/autoseed          core: options, plan, report, errors
github.com/danellalc/autoseed/gormseed adapter: GORM
github.com/danellalc/autoseed/entseed  adapter: ent (v2)
github.com/danellalc/autoseed/cmd/autoseed  CLI (explain, later capture)
```

Core knows no ORM. Adapters know no CLI. The user imports one adapter and the option types from the root — nothing else.

## Pipeline

Seven stages, same discipline as EFCore.AutoSeed. New code belongs to exactly one.

```
ORM model (via adapter)
  1. ModelReader        entities, fields, keys, FKs, associations, soft-delete, embedded
  2. DependencyGraph    stable topological sort (ties break on entity name)
  3. CycleResolver      nullable cycles → two passes; required cycles → named error
  4. GenerationPlan     row counts, cardinality, distribution shapes
  5. ValueGeneration    semantic inference over gofakeit, seeded
  6. ConstraintSatisfaction  unique, not-null, length, enum-like sets
  7. Persistence        ordered insert, batched
```

Stages 2–7 live in the core and are adapter-independent. Stage 1 is the adapter contract:

```go
type ModelSource interface {
    Entities() ([]Entity, error)
}
```

An `Entity` carries fields, keys, and typed references to other entities. Everything downstream consumes only this — which is what makes the ent adapter (and any future one) a stage-1 job, not a rewrite.

---

## The hard problems

### GORM's model is convention over declaration

EF Core has one `IModel` with everything stated. GORM infers from struct tags, field names (`ID`, `CreatedAt`, `DeletedAt`), and conventions — `schema.Parse` exposes the resolved schema, but corners exist:

- **Associations by convention**: `UserID uint` next to `User User` is a belongs-to even with no tag. The reader must use GORM's own resolution (`schema.Relationships`), never re-derive it.
- **Soft delete**: `gorm.DeletedAt` means every query is filtered. Generating 50% deleted rows makes the app see nothing — generate the overwhelming majority live, small configurable proportion deleted. (Same lesson as EF query filters.)
- **Polymorphic associations** (`polymorphic:"Owner"`): supported reading, generation deferred to v2 — named as skipped in Explain, never silently wrong.
- **Embedded structs** (`gorm:"embedded"`): columns on the owner, not entities. Treating them as tables is the mistake that breaks everything (owned-types lesson, verbatim).

### Cycles

- Nullable FK in cycle → insert nil, second pass `UPDATE`.
- Required FK cycle → `ErrUnsatisfiableCycle` naming every entity in the cycle. Failing clearly is the feature.
- Self-reference (`ManagerID *uint`) → nullable path; required self-reference → error.

### Uniqueness without O(n²)

Pre-shuffled pools with uniqueness by construction; bounded retry; deterministic suffix as last resort. Ported design, not rediscovered.

### Determinism in Go

Same seed, same data, always — the central guarantee.

- All randomness flows through a `*rand.Rand` seeded from the root (gofakeit v7 accepts a source; `gofakeit.NewFaker(rand.NewPCG(seed, seed), false)`).
- Derivation is hierarchical and positional: `root → entity → row index → field`. Row 500 alone equals row 500 in a batch.
- **Map iteration order is random in Go by design.** Any map whose iteration affects output must be sorted first. This is the #1 determinism trap in the port and has a dedicated test.
- Generation is sequential; insertion may be batched. Any change that alters generated data for a given seed is a breaking change (major version).

### database/sql and batching

`CreateInBatches` for GORM path. Respect `autoIncrement` — read back generated IDs before children generate FKs. For PostgreSQL, `RETURNING` covers it; MySQL needs `LastInsertId` arithmetic on batches — sharp edge, tested against real databases, not SQLite-only.

### The InMemory lesson, Go edition

SQLite in-memory is Go's InMemory trap: it accepts what MySQL and PostgreSQL reject (constraint enforcement differences, type affinity). Integration tests run against real engines via testcontainers-go. SQLite is a supported *target*, never the only test surface.

---

## Design decisions

**Why the ORM model instead of DDL introspection.** Same as the original: DDL sees tables; the model sees associations, embedded structs, soft-delete intent. Generating from the model produces data the *application* can read. Works before the database exists. SynthDB and Seedfast read DDL and are PostgreSQL-only — this is the structural difference, stated in the README.

**Why gofakeit stays.** It is the ecosystem's value engine (300+ functions, locales, seeded API). autoseed is the layer it documents as out of scope: consistent relationships. Building on it inherits credibility and locale data; replacing it would be scope creep with no upside.

**Why GORM first, ent second.** GORM has the largest install base — distribution beats elegance for v1. ent's schema is literally a graph exposed by codegen (`gen.Graph`), making it the cheapest adapter to add and the best showcase of the adapter contract — perfect v2.

**Why functional options.** `autoseed.WithSeed(42)` is the Go-idiomatic configuration pattern; a config struct would fossilize defaults. Options live in the root package so adapters share them.

**Why errors, not panics.** Library code returns typed errors (`ErrUnsatisfiableCycle`, `ErrUnsupportedField`) wrapping context via `%w`. `errors.Is`/`errors.As` friendly. Panic in a library is an instant issue.

---

## Roadmap

**v1 — GORM core**
Model reading via `schema.Parse`, stable topological sort, nullable-cycle resolution, ~15 inference rules with coherence (FirstName+LastName+Email agree; UpdatedAt ≥ CreatedAt), basic long tail, deterministic seed, PostgreSQL + MySQL + SQLite, `Seed`, `Explain`, `SeedCoverage`.

**v2 — depth**
ent adapter (proves the contract). Polymorphic generation. Temporal clustering, null rates, dirty-data mode. `autoseed explain` CLI. Benchmarks.

**v3 — shape**
Row-count capture from `pg_class.reltuples` / `information_schema.tables` and proportional apply — the production-shape feature, ported.

**Out of scope, permanently until traction says otherwise:** anonymisation, cloud, raw `database/sql` support, speculative adapters. Two tools in this category died of scope creep; the boundary is the survival strategy.
