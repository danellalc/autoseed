# Architecture

## Packaging

One module, adapters as subpackages. **No separate repos, no separate modules** — one `go get` brings everything, and the adapter you don't import costs you nothing.

```
github.com/danellalc/autoseed          core: options, plan, report, errors — ships today
github.com/danellalc/autoseed/inference value generation, gofakeit-backed — ships today
github.com/danellalc/autoseed/gormseed adapter: GORM — ships today
github.com/danellalc/autoseed/entseed  adapter: ent — planned, v2
github.com/danellalc/autoseed/cmd/autoseed  CLI (explain, later capture) — planned, v2
```

`examples/` holds runnable, separately-moduled demo programs, one per adapter — not part of the public API.

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
- **One-directional `HasMany`/`HasOne`**: a parent declaring `Items []Item` with no reciprocal field on `Item` is the common case, and `Item`'s own schema carries no `BelongsTo` entry for it at all — the reference only shows up from the parent's side, attributed to the child via `Relationship.FieldSchema`. Reading `BelongsTo` alone silently drops the single most common foreign key shape in real models.
- **`SetupJoinTable`**: an explicit many2many join struct with its own extra column (a membership role, a friendship date) is only visible if parsing goes through the same `*gorm.DB`'s own schema cache — a fresh `sync.Map` per read sees only the bare two-FK table GORM synthesizes by default, silently dropping the extra column.
- **Composite `references:` order**: GORM keeps a composite foreign key's columns in the tag's own declared order, not the target's primary key declaration order. The two are not guaranteed to match, and a Reference's `Fields` must be reordered against the target's own primary key fields, not trusted from the tag, or downstream stages pairing them positionally get the wrong column.
- **Soft delete**: `gorm.DeletedAt` means every query is filtered. Generating 50% deleted rows makes the app see nothing — generate the overwhelming majority live, small proportion deleted. (Same lesson as EF query filters, same 90/10 default split.) Detected via `schema.QueryClausesInterface`, the mechanism GORM itself uses, never by matching the field name — `inference.SoftDeleteRule` reads `Field.SoftDelete`, not the column's name or type, so it fires on any GORM-recognized soft-delete field. The rate is fixed for v1; a configurable knob, like null rate, is a v2 item.
- **Polymorphic associations** (`polymorphic:"Owner"`): supported reading, generation deferred to v2 — named as skipped in Explain, never silently wrong.
- **Embedded structs** (`gorm:"embedded"`): columns on the owner, not entities. Treating them as tables is the mistake that breaks everything (owned-types lesson, verbatim).

### Cycles

- Nullable FK in cycle → insert nil, second pass `UPDATE`.
- Required FK cycle → `ErrUnsatisfiableCycle` naming every entity in the cycle. Failing clearly is the feature.
- Self-reference (`ManagerID *uint`) → nullable path; required self-reference → error.

### Uniqueness without O(n²)

Not a pre-shuffled pool — checked the actual `.NET` source (`UniquenessEnforcer.cs`) rather than trust its own README, and no such pool exists there either, only in the prose. The real, shipped algorithm, ported as-is: generate every row first, then for each single-column string field marked unique, scan in row order with a `map[string]bool` of values seen; the first row to use a value keeps it, a duplicate gets a random numeric suffix, retried up to 20 times before giving up named. Composite unique constraints and non-string columns are out of scope, same as the original.

### Determinism in Go

Same seed, same data, always — the central guarantee.

- All randomness flows through a `*rand.Rand` seeded from the root (gofakeit v7 accepts a source; `gofakeit.NewFaker(rand.NewPCG(seed, seed), false)`).
- Derivation is hierarchical and positional: `root → entity → row index → field`. Row 500 alone equals row 500 in a batch.
- **Map iteration order is random in Go by design.** Any map whose iteration affects output must be sorted first. This is the #1 determinism trap in the port and has a dedicated test.
- Generation is sequential; insertion may be batched. Any change that alters generated data for a given seed is a breaking change (major version).

### database/sql and batching

`CreateInBatches` for GORM path, one entity type at a time in dependency order: generate its rows, insert, and only then move to the next entity type, so a child's foreign keys are assigned from the parent's real, already-inserted primary key — never a value invented independently of what actually got written. For PostgreSQL, `RETURNING` covers the read-back; MySQL needs `LastInsertId` arithmetic on batches — verified against a real container, not SQLite-only.

**A table with no column GORM will actually list in the INSERT statement cannot be trusted with a multi-row batch.** Confirmed against real PostgreSQL, independent of this package: `INSERT INTO t DEFAULT VALUES` (what GORM emits when every column is an auto-increment field GORM omits) has no multi-row form, so a `CreateInBatches` call over N such rows reports back the first row's generated id and silently leaves the rest at their zero value — no error, just wrong foreign keys downstream. The simplest possible lookup entity — one auto-increment primary key, nothing else — hits this. A many2many join table does *not*: both its key columns are ordinary foreign keys, not auto-increment, so GORM lists real values for both and batches normally. The fix checks `AutoIncrement`, not `PrimaryKey` — batch size of 1 only when every field is auto-increment; everything else, including a composite key of non-auto-increment columns, keeps the real batch size. (An earlier version of this fix checked `PrimaryKey` instead and silently serialized every many2many insert to one row at a time — found by review, not by a failing test, since the wrong condition never produced wrong data, only wasted round-trips.)

### The InMemory lesson, Go edition

SQLite in-memory is Go's InMemory trap: it accepts what MySQL and PostgreSQL reject (constraint enforcement differences, type affinity). Integration tests run against real engines via testcontainers-go. SQLite is a supported *target*, never the only test surface.

---

## Design decisions

**Why the ORM model instead of DDL introspection.** Same as the original: DDL sees tables; the model sees associations, embedded structs, soft-delete intent. Generating from the model produces data the *application* can read. Works before the database exists. SynthDB and Seedfast read DDL and are PostgreSQL-only — this is the structural difference, stated in the README.

**Why gofakeit stays.** It is the ecosystem's value engine (300+ functions, seeded API). autoseed is the layer it documents as out of scope: consistent relationships. Building on it inherits credibility; replacing it would be scope creep with no upside.

gofakeit has no locale support at all (v7.15.0, checked directly against its source — no per-locale data, no CPF/CNPJ, English only), unlike Bogus on the .NET side. `inference.DocumentRule` hand-rolls the Brazilian CPF/CNPJ check-digit algorithm (mod 11), ported from the .NET sibling's `BrazilianDocuments.cs`, because there is nothing to call into. Any future locale-specific rule will need the same treatment; do not assume gofakeit covers it.

**Why GORM first, ent second.** GORM has the largest install base — distribution beats elegance for v1. ent's schema is literally a graph exposed by codegen (`gen.Graph`), making it the cheapest adapter to add and the best showcase of the adapter contract — perfect v2.

**Why functional options.** `autoseed.WithSeed(42)` is the Go-idiomatic configuration pattern; a config struct would fossilize defaults. Options live in the root package so adapters share them.

**Why errors, not panics.** Library code returns typed errors (`ErrUnsatisfiableCycle`, `ErrUnsupportedField`) wrapping context via `%w`. `errors.Is`/`errors.As` friendly. Panic in a library is an instant issue.

**Why a required non-driver target at zero rows pads instead of erroring.** The .NET sibling throws a named exception when this happens (`Persistence.cs`'s `UnsupportedEntityTypeException`); Go instead floors that one driver row's child count to 1 (`generation_plan.go`'s `requiredReferenceTargets` check) and keeps going. Both avoid the real failure mode — a raw FK constraint violation — but land on opposite answers to "what should a caller see." autoseed's whole value proposition is `Seed(ctx, db, models, WithScale(1))` just working without the caller hunting for a seed/scale combination that dodges an edge case; throwing here would occasionally turn an ordinary small-scale request into an inexplicable failure. A deliberate divergence, not an unreconciled one — flagged in review as looking accidental, kept anyway, documented here so the next person doesn't "fix" it back to matching .NET without reading this paragraph first.

---

## Roadmap

**v1 — GORM core**
Model reading via `schema.Parse`, stable topological sort, nullable-cycle resolution, ~16 inference rules with coherence (FirstName+LastName+Email agree; UpdatedAt ≥ CreatedAt; Total = Price × Quantity) and a biased soft-delete rate, basic long tail, deterministic seed, `Seed` and `Explain` — shipped, tested against real PostgreSQL and MySQL, plus a combined "MegaMart" model and referential-integrity property tests exercising every shape together. `SeedCoverage` still open; the property tests and the combined "MegaMart" model run against PostgreSQL only — MySQL's own coverage is a required-FK chain and its `LastInsertId` batch arithmetic, not the same depth; SQLite runs `Seed` and `Explain` in the example and the package's own `Example` tests, narrower still. Composite unique constraints and non-auto-increment (e.g. UUID) primary keys are not generated — same scope the .NET sibling ships with.

**v2 — depth**
ent adapter (proves the contract). Polymorphic generation. Temporal clustering, null rates, dirty-data mode. `autoseed explain` CLI. Benchmarks.

**v3 — shape**
Row-count capture from `pg_class.reltuples` / `information_schema.tables` and proportional apply — the production-shape feature, ported.

**Out of scope, permanently until traction says otherwise:** anonymisation, cloud, raw `database/sql` support, speculative adapters. Two tools in this category died of scope creep; the boundary is the survival strategy.
