# Architecture

## Packaging

One module, adapters as subpackages. **No separate repos, no separate modules** — one `go get` brings everything, and the adapter you don't import costs you nothing.

```
github.com/danellalc/autoseed          core: options, plan, report, errors — ships today
github.com/danellalc/autoseed/inference value generation, gofakeit-backed — ships today
github.com/danellalc/autoseed/gormseed adapter: GORM — ships today
github.com/danellalc/autoseed/entseed  adapter: ent — ships today
github.com/danellalc/autoseed/cmd/autoseed  CLI (explain, later capture) — planned
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
- **Soft delete**: `gorm.DeletedAt` means every query is filtered. Generating 50% deleted rows makes the app see nothing — generate the overwhelming majority live, small proportion deleted. (Same lesson as EF query filters, same 90/10 default split.) Detected via `schema.QueryClausesInterface`, the mechanism GORM itself uses, never by matching the field name — `inference.SoftDeleteRule` reads `Field.SoftDelete`, not the column's name or type, so it fires on any GORM-recognized soft-delete field. The rate itself is still fixed, not a configurable knob — `WithNilRate` (below) covers ordinary Nullable fields, deliberately not this one, since SoftDeleteRule already owns a more deliberate, CreatedAt/UpdatedAt-coherent split a blind rate roll would bypass.
- **Polymorphic associations** (`polymorphic:"Owner"`): supported reading, generation deferred to v2 — named as skipped in Explain, never silently wrong.
- **Embedded structs** (`gorm:"embedded"`): columns on the owner, not entities. Treating them as tables is the mistake that breaks everything (owned-types lesson, verbatim).
- **`database/sql` nullable wrappers** (`sql.NullString`, `sql.NullInt64`, `sql.NullBool`, `sql.NullFloat64`, `sql.NullTime`, and the narrower `NullInt32`/`NullInt16`/`NullByte`), and a Go pointer field: an extremely common, idiomatic way to model a nullable column, and (for the wrapper types) a `reflect.Struct` kind no rule's `CanInfer` would ever match on its own — `GenerateRow` unwraps a field of one of these types to its wrapped/pointed-to type before running the normal rule pass (so a field named `Email` of type `sql.NullString` still gets `EmailRule`'s treatment, not just generic text), then rewraps the result. `autoseed.WithNilRate(rate)` sets the per-row probability such a field is left out entirely instead, producing a real database NULL — gated on `Field.Nullable`, which gormseed only sets true when the field's Go representation can actually hold a nil (a pointer, or one of these wrapper types): `Field.Set(ctx, instance, nil)` on a plain, non-pointer field returns no error but silently writes that type's zero value, not a real NULL, so a schema-nullable-but-Go-inflexible column is never subject to the roll. ent has no such gate — its `ClearField` mutation method works uniformly for any `Optional()` field regardless of Go representation.

### Composite and shared primary keys

Two shapes let a child's primary key be built entirely, or partly, from its parent's: a **composite-key junction** (a many-to-many join table, or an explicit "attributed join" entity like an inventory or order-line row, whose primary key — or, for ent, whose surrogate ID plus a composite `UniqueConstraints` entry, since ent has no composite primary keys — is exactly its driving reference's and one or more other required references' foreign key columns together) and a **shared-primary-key one-to-one** (GORM's equivalent of EF Core table splitting — a child whose primary key IS its only foreign key, e.g. `ProductProfile.ProductID`). Left uncapped, either shape's ordinary Exponential long-tail draw can ask a driver row for more children than the other participants have distinct combinations to pair with, which cannot be satisfied without two children sharing a key.

`generation_plan.go`'s `junctionCap` and `sharesDriverPrimaryKey` detect both shapes from `Entity`/`Reference`/`Field` alone — no adapter-specific knowledge — and cap the draw: at the product of every non-driver participant's own row count for a junction (one factor per participant, so the original pairwise two-participant shape is just the one-factor case), at exactly one for a shared key. Matching is by each reference's own foreign key **fields**, never by target name alone, because a **self-referencing many-to-many** (both foreign keys targeting the same entity, e.g. a `Person` "follows" `Person`) gives two references an identical target; only their own columns (`FollowerID` vs `FollowingID`) tell them apart. A self-referencing participant also lowers its own factor by one (a row can never pair with itself), and `autoseed.JunctionIndices` — shared by both adapters' persistence code, rather than duplicated — decomposes a block-local position in mixed radix over each participant's own factor, so every row in a driver's block draws a distinct combination across all participants, with the self-referencing offset (`+driverRow+1, mod N`) applied per participant that needs it.

A ternary or higher-degree "attributed join" (three or more required references together completing the key) is the N-participant case of the same mechanism, not a separate one: `junctionCap` collects however many required references exactly cover the key's remaining fields once the driver's own fields are removed, in any number, not just one.

A primary key field that is neither an auto-increment column nor covered by any reference — the one natural, non-FK part of an otherwise reference-driven composite key (an `EffectiveDate` alongside `WarehouseID`+`ProductID`), or a whole natural key on a standalone entity — is generated by `inference.GenerateRow` like any other field now, instead of left at its Go zero value. This narrows, but doesn't erase, the natural-key gap below: it only helps when some rule recognizes the field's Go type.

### Cycles

- Nullable FK in cycle → insert nil, second pass `UPDATE`.
- Required FK cycle → `ErrUnsatisfiableCycle` naming every entity in the cycle. Failing clearly is the feature.
- Self-reference (`ManagerID *uint`) → nullable path; required self-reference → error.

### Uniqueness without O(n²)

Not a pre-shuffled pool — checked the actual `.NET` source (`UniquenessEnforcer.cs`) rather than trust its own README, and no such pool exists there either, only in the prose. The real, shipped algorithm, ported as-is: generate every row first, then for each single-column string field marked unique, scan in row order with a `map[string]bool` of values seen; the first row to use a value keeps it, a duplicate gets a random numeric suffix, retried up to 20 times before giving up named. A composite unique constraint (GORM's `uniqueIndex` tag; ent's `index.Fields`/`index.Edges`) with at least one independent string column gets the same treatment on its tuple, rewriting the last string field in the constraint's own order; two constraints sharing a rewritable field are resolved together in one pass per row, not independently, since fixing one can change what the row's tuple looks like under the other. A constraint naming only foreign key columns is left alone here — a row's reference fields hold only a placeholder at this stage, not the real parent key persistence assigns later — and is `junctionCap`'s job instead, described above. Non-string columns beyond that FK case are still out of scope, same as the original.

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

gofakeit has no locale support at all (v7.15.0, checked directly against its source — no per-locale data, no CPF/CNPJ, English only), unlike Bogus on the .NET side. `inference.DocumentRule` hand-rolls the Brazilian CPF/CNPJ check-digit algorithm (mod 11), ported from the .NET sibling's `BrazilianDocuments.cs`, because there is nothing to call into. `WithLocale("pt_BR")`'s own name/city/street/phone rules got the same treatment: small, hand-rolled ASCII pools and a Brazilian mobile-number format, registered ahead of the generic rules (Priority -1) rather than reaching for a locale gofakeit doesn't have. Any future locale-specific rule will need the same treatment; do not assume gofakeit covers it.

**Why GORM first, ent second.** GORM has the largest install base — distribution beats elegance for v1. ent's schema is literally a graph exposed by codegen (`gen.Graph`), making it the cheapest adapter to add and the best showcase of the adapter contract — it has since shipped, on the same real-Postgres testing bar as GORM.

**Why functional options.** `autoseed.WithSeed(42)` is the Go-idiomatic configuration pattern; a config struct would fossilize defaults. Options live in the root package so adapters share them.

**Why errors, not panics.** Library code returns typed errors (`ErrUnsatisfiableCycle`, `ErrUnsupportedField`) wrapping context via `%w`. `errors.Is`/`errors.As` friendly. Panic in a library is an instant issue. Every exported entry point that takes a `*SeededSource` or an `Options` validates it before use — `PlanGeneration` and `EnsureUnique` return `ErrNilSeed` for a nil seed rather than dereferencing it, and `PlanGeneration` returns `ErrInvalidScale` for a negative `Options.Scale` rather than handing it to `make([]int, n)`. `gormseed.Seed` returns `ErrNilDB` for a nil `*gorm.DB` the same way, rather than reaching into it and panicking inside GORM.

**Why a required non-driver target at zero rows pads instead of erroring.** The .NET sibling throws a named exception when this happens (`Persistence.cs`'s `UnsupportedEntityTypeException`); Go instead floors that one driver row's child count to 1 (`generation_plan.go`'s `requiredReferenceTargets` check) and keeps going. Both avoid the real failure mode — a raw FK constraint violation — but land on opposite answers to "what should a caller see." autoseed's whole value proposition is `Seed(ctx, db, models, WithScale(1))` just working without the caller hunting for a seed/scale combination that dodges an edge case; throwing here would occasionally turn an ordinary small-scale request into an inexplicable failure. A deliberate divergence, not an unreconciled one — flagged in review as looking accidental, kept anyway, documented here so the next person doesn't "fix" it back to matching .NET without reading this paragraph first.

---

## Roadmap

**v1 — GORM and ent core**
Model reading via `schema.Parse` (GORM) and `entc.LoadGraph` (ent), stable topological sort, nullable-cycle resolution (minimal-edge, every independent unsatisfiable cycle named), ~16 inference rules with coherence (FirstName+LastName+Email agree; UpdatedAt ≥ CreatedAt; DeletedAt ≥ both; Total = Price × Quantity) and a biased soft-delete rate, `database/sql` nullable-wrapper and pointer-field support with `WithNilRate` for a configurable null probability, `WithLocale("pt_BR")` for Brazilian name/city/street/phone generation, cardinality capping for composite and shared primary keys (including self-referencing joins and ternary/N-ary attributed joins), single-column and composite unique-field dedup, basic long tail, deterministic seed, `Seed` and `Explain` on both adapters — shipped, tested against real PostgreSQL and MySQL, plus a combined "MegaMart" model and referential-integrity property tests exercising every shape together (GORM) and real end-to-end Postgres tests per shape (ent). `SeedCoverage` still open; the property tests and the combined "MegaMart" model run against PostgreSQL only — MySQL's own coverage is a required-FK chain and its `LastInsertId` batch arithmetic, not the same depth; SQLite runs `Seed` and `Explain` in the example and the package's own `Example` tests, narrower still. A many-to-many edge on either adapter (ent's edges, GORM's polymorphic associations), and a non-auto-increment primary key of a type no rule recognizes, are not generated — named as skipped or `ErrUnsupportedField`, or a language-level limit (Go can't enumerate a named type's constants the way C#'s `Enum.GetValues` can).

**v2 — depth**
Polymorphic and many-to-many generation. A second locale. Temporal clustering, dirty-data mode. `autoseed explain` CLI. Benchmarks.

**v3 — shape**
Row-count capture from `pg_class.reltuples` / `information_schema.tables` and proportional apply — the production-shape feature, ported.

**Out of scope, permanently until traction says otherwise:** anonymisation, cloud, raw `database/sql` support, speculative adapters. Two tools in this category died of scope creep; the boundary is the survival strategy.
