# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[Semantic Versioning](https://semver.org/) — in Go, a major bump also changes
the import path, so v1.0.0 is held back deliberately until the API settles.

Any change that alters the data generated for a given seed is a breaking
change, even if the public API itself is unchanged.

## [Unreleased]

### Added

- `entseed`, the ent adapter: `entseed.Explain` and `entseed.Seed`, reading a
  schema via `entc.LoadGraph` (ent exposes its resolved graph only through
  the same mechanism `entc` itself uses during `go generate`, not runtime
  introspection the way GORM's `schema.Parse` works on a live `*gorm.DB`) —
  real inserts against real PostgreSQL, tested end to end: a required chain,
  a nullable self-reference, a many-to-many edge reported once as a named
  skip in the `Explain` report rather than silently dropped (ent has no
  addressable join-table entity for one, unlike GORM's auto-generated join
  struct), a composite unique index over plain fields and over edge-owned
  foreign keys, and a ternary attributed join expressed as a surrogate ID
  plus a composite `UniqueConstraints` entry, since ent has no composite
  primary keys. `entseed.Seed` returns `entseed.ErrNilClient` for a nil
  client rather than panicking.
- Composite unique constraints, on both adapters: `Entity.UniqueConstraints`
  holds a field-name tuple per composite index (GORM's `uniqueIndex` tag;
  ent's `index.Fields`/`index.Edges`). `EnsureUnique` dedups a tuple with at
  least one independent, non-foreign-key string column by rewriting its last
  such field on collision; two constraints sharing a rewritable field are
  resolved together in one pass per row, not independently, so fixing one
  can never silently reopen a duplicate under the other. A constraint
  naming only foreign key columns is left to the generation plan's
  cardinality cap instead, described next.
- The junction cardinality cap generalizes from exactly one non-driver
  participant to any number: a composite key — the entity's own primary
  key, or now also a `UniqueConstraints` entry spanning foreign keys —
  covered by a driver reference plus N other required references caps each
  driver row's child count at the product of those references' own row
  counts, not just a single factor. `autoseed.JunctionIndices`, shared by
  both adapters instead of duplicated, maps a block-local position to a
  distinct per-participant row index via mixed-radix decomposition; the
  original pairwise shape (a many-to-many join table, an "attributed join"
  entity) is the one-participant case of the same mechanism, so it keeps
  working unchanged.
- `autoseed.WithNilRate(rate)`: the per-row probability a `Nullable` field's
  value is left out entirely, producing a real database NULL, instead of
  always generating one. `Field.Nullable` now means "generation can
  actually leave this out and have persistence turn it into a real NULL,"
  not just "the column's schema allows NULL" — gormseed only sets it true
  for a pointer field or a `database/sql` nullable wrapper, since
  `Field.Set(ctx, instance, nil)` silently writes a plain field's zero
  value, not a real NULL, for anything else; ent's generic `ClearField`
  mutation method works uniformly for any `Optional()` field regardless of
  Go representation, so entseed needed no such gate. A field also flagged
  `SoftDelete` is excluded from this generic roll, since `SoftDeleteRule`
  already owns that field's own, more deliberate null-vs-not split. The
  roll is derived from a seed position distinct from whatever randomness a
  surviving field's own rule draws, so surviving a rate below 1.0 never
  biases that rule's own output toward a particular range.
- Fixed a pre-existing entseed bug this work surfaced: `createRows` set a
  mutation field by its declared ent name, but ent's generated `SetField`
  matches on the field's storage key, which only happened to be textually
  identical to the name in every schema used before a fixture added an
  explicit `StorageKey` override. `Seed` now translates through each
  entity's own name-to-storage-key map before calling `SetField`.
- `autoseed.WithLocale(locale)`: swaps in a locale's own name/city/street/
  phone rules ahead of the generic ones, `Priority` -1 so they always claim
  first. `pt_BR` is the first (and, today, only) locale: a `FirstName`/
  `LastName` pair from a small real-name pool, a real Brazilian city or a
  "`Rua`/`Avenida`/`Travessa`/`Alameda` <name>" street, and a
  `+55 (DDD) 9XXXX-XXXX` mobile number from real area codes — all
  hand-rolled, ASCII only (no diacritics), since gofakeit itself has no
  locale data to call into, the same reason `DocumentRule`'s CPF/CNPJ
  algorithm is hand-rolled. `EmailRule` needed no changes at all: it
  already builds an address from whatever `FirstName`/`LastName` landed on
  the same row, with no locale awareness of its own, so a Brazilian name
  flows through it for free. An unrecognized locale, including not calling
  `WithLocale`, falls back to the generic rules — never an error, matching
  `WithNilRate`'s own silent-clamp convention.
- Fixed a real, previously-undiscovered entseed bug this work surfaced:
  `buildEntity` set `Field.Name` to an ent field's own declared name
  verbatim — "first_name" for a schema author's idiomatic
  `field.String("first_name")` — but every named inference rule matches a
  PascalCase suffix like "FirstName" or "CreatedAt", the convention a GORM
  struct field name always already follows. A snake_case name never
  matches such a suffix as a continuous string, so for any idiomatically-
  named ent schema, every named rule silently never fired: `Email` would
  never have agreed with `FirstName`/`LastName`, `CreatedAt`/`UpdatedAt`
  coherence would never have applied. Every field name (and the composite
  unique index and storage-key maps keyed by it) is now pascal-cased the
  same way an edge's own `Reference.Fields` name already was.

## [0.1.0]

First tagged release. Reads a GORM model and writes referentially valid,
deterministic rows — no hand-written ordering, no fixtures.

### Added

- `gormseed.Explain` — reads a GORM model via `schema.Parse`, resolves
  insertion order and cycles, returns a `Plan` without writing anything.
- `gormseed.Seed` — writes real rows in dependency order: required foreign
  keys copied from an already-inserted parent's real primary key, nullable
  cycles patched in a second, deferred pass. Returns `gormseed.ErrNilDB`
  for a nil `db` rather than reaching into it.
- Deterministic generation: every random draw derives from one root seed,
  hierarchically and positionally (root → entity → row index → field). Same
  seed, same data, always — verified by a dedicated test, including an
  anti-map-iteration guard.
- Stable topological sort (ties break on entity name) and Tarjan-based cycle
  detection. Breaking a cycle defers only the one nullable edge needed to
  make it acyclic, never every nullable edge in it, so an entity keeps
  driving its cardinality off any reference a cycle didn't actually need
  broken. A required cycle with no nullable edge to break it returns
  `ErrUnsatisfiableCycle`, naming the real cycle path; every independent
  unsatisfiable cycle in a model is named in one error, not just the first
  found.
- `autoseed.GenerationPlan`: long-tail cardinality (Exponential distribution)
  for related row counts, one driving principal per dependent entity
  (identified by its own foreign key columns, not just its target's name, so
  two references to the same entity — a self-referencing many-to-many — are
  never confused with each other), a required non-driver reference target is
  never left at zero rows. A composite-primary-key "attributed join" entity
  (a many-to-many table with extra columns, or GORM's own auto-generated join
  table) caps each driver row's child count at its other side's row count —
  one less, and never at itself, when the join is self-referencing — and a
  shared-primary-key one-to-one (the GORM equivalent of EF Core's table
  splitting) caps at exactly one; either shape, uncapped, risks two children
  colliding on the same primary key. The cap still finds the right pair of
  references even when a third, unrelated required reference sits on the
  same entity. A composite primary key of three or more foreign key columns
  (a ternary relationship) is out of scope for this pairwise check and stays
  uncapped — see Known gaps. `PlanGeneration` returns `ErrInvalidScale` for a
  negative `Options.Scale` and `ErrNilSeed` for a nil seed, rather than
  panicking on a negative slice length or a nil dereference.
- ~16 built-in `inference` rules over gofakeit v7 (seeded): name/email/phone/
  address/URL, coherent `CreatedAt`/`UpdatedAt` and `Price`×`Quantity`=`Total`
  on the same row, a hand-rolled Brazilian CPF/CNPJ check-digit generator (no
  locale mechanism yet — matches on field name alone), and a soft-delete rule
  that leaves `gorm.DeletedAt` unset on 90% of rows and, for the rest, a
  timestamp no earlier than the row's own `CreatedAt`/`UpdatedAt`. Every
  rule's string output is truncated to the column's declared size, by rune
  not by byte (a byte-index cut could split a multi-byte character and hand
  the database invalid UTF-8), before it reaches the database — not just the
  generic fallback rule's. A primary key field that is neither an
  auto-increment column nor a foreign key — the one natural, non-reference
  part of a mixed composite key, or a whole natural key on a standalone
  entity — is generated like any other field instead of left at its Go zero
  value. A field typed as one of `database/sql`'s nullable wrappers
  (`sql.NullString`, `sql.NullInt64`, `sql.NullBool`, `sql.NullFloat64`,
  `sql.NullTime`, and the narrower `NullInt32`/`NullInt16`/`NullByte`) is
  matched against rules by its wrapped value's type — a field named `Email`
  of type `sql.NullString` still gets `EmailRule`'s treatment, not just
  generic text — and always comes back `Valid`; there is no null-rate knob
  yet.
- `autoseed.EnsureUnique`: duplicate values for a single-column string unique
  field are rewritten with a random numeric suffix, retried up to 20 times,
  by rune not by byte, before `ErrUnsatisfiableUniqueness` names the field.
  Returns `ErrNilSeed` for a nil seed rather than panicking.
- Embedded structs (`gorm:"embedded"`) generated as columns on the owner,
  never as their own entity. Polymorphic associations are read and skipped
  by name in the `Explain` report — generation is a v2 item.
- Property-based tests (`pgregory.net/rapid`): the graph/cycle engine for any
  generated model and seed, and `gormseed.Seed` itself against real
  PostgreSQL for a linear chain, a diamond of two required principals, a
  nullable self-reference, a composite-primary-key junction, a
  shared-primary-key one-to-one, and a self-referencing many-to-many (both
  foreign keys targeting the same entity, e.g. a "follows" table) — no
  orphaned foreign keys, no primary key collisions, and no accidental
  self-loops, ever.
- `examples/gormseed-basic`, a runnable example (its own Go module, `replace`
  points at this checkout) seeding a real SQLite database end to end.
- `Seed` accepts `ctx` and now actually honors it: a canceled or
  deadline-exceeded context stops in-flight batch inserts and the deferred
  second-pass transaction, not just the in-memory value generation.
- A many-to-many join table, or any other composite-primary-key entity
  made entirely of non-auto-increment columns, batches inserts at the
  normal size instead of one row at a time — the single-row fallback is
  now scoped to what actually needs it (a table where every column is
  auto-increment), not every primary-key-only table.

### Known gaps

- `gormseed.SeedCoverage` (the smallest dataset that exercises everything) is
  not built.
- Composite unique constraints (secondary indexes spanning more than one
  column) are not generated.
- A non-auto-increment primary key with no reference driving it at all — a
  standalone entity's own natural key, e.g. an app-generated UUID — is
  generated using whatever rule matches its Go type, same as any other
  field; a type no rule recognizes (a `uuid.UUID`, a custom `driver.Valuer`)
  fails named with `ErrUnsupportedField` rather than being silently left at
  its zero value.
- A composite primary key of three or more foreign key columns (a ternary
  relationship) is not cardinality-capped — see `autoseed.GenerationPlan`
  above.
- A Go named-integer "enum" type generates a plain unconstrained number, not
  one of its declared constants — Go's reflection cannot enumerate them the
  way C#'s `Enum.GetValues` can.
- `PriceRule` doesn't match `Balance` and isn't precision/scale-aware
  (`Field` carries no `Precision`/`Scale`); there is no `Discount`/
  `AmountDue`-style correlated rule.
- The deferred (cycle-broken) second pass updates one row at a time, not
  batched — every other write path batches at 500 rows. Correct, not a
  scaling match for the rest of the pipeline; an entity with a
  self-reference or a broken cycle pays one round trip per row in that pass.
- The property tests and the "MegaMart" model run against PostgreSQL only;
  MySQL's own coverage is a required-FK chain and its `LastInsertId` batch
  arithmetic, not the same depth. SQLite runs `Seed`/`Explain` in the
  package's own `Example` tests and the runnable example, narrower still.
- `entseed` (ent adapter), a CLI, and every value-realism knob beyond the
  built-in rules (locale, null rate, dirty data, temporal clustering) are
  not built — see [ARCHITECTURE.md](ARCHITECTURE.md#roadmap).
