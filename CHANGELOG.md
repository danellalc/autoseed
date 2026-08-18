# Changelog

All notable changes to this project are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[Semantic Versioning](https://semver.org/) — in Go, a major bump also changes
the import path, so v1.0.0 is held back deliberately until the API settles.

Any change that alters the data generated for a given seed is a breaking
change, even if the public API itself is unchanged.

## [Unreleased]

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
  for related row counts, one driving principal per dependent entity, a
  required non-driver reference target is never left at zero rows. A
  composite-primary-key "attributed join" entity (a many-to-many table with
  extra columns, or GORM's own auto-generated join table) caps each driver
  row's child count at its other side's row count, and a shared-primary-key
  one-to-one (the GORM equivalent of EF Core's table splitting) caps at
  exactly one — either shape would otherwise risk two children colliding on
  the same primary key.
- ~16 built-in `inference` rules over gofakeit v7 (seeded): name/email/phone/
  address/URL, coherent `CreatedAt`/`UpdatedAt` and `Price`×`Quantity`=`Total`
  on the same row, a hand-rolled Brazilian CPF/CNPJ check-digit generator (no
  locale mechanism yet — matches on field name alone), and a soft-delete rule
  that leaves `gorm.DeletedAt` unset on 90% of rows and, for the rest, a
  timestamp no earlier than the row's own `CreatedAt`/`UpdatedAt`. Every
  rule's string output is truncated to the column's declared size before it
  reaches the database, not just the generic fallback rule's.
- `autoseed.EnsureUnique`: duplicate values for a single-column string unique
  field are rewritten with a random numeric suffix, retried up to 20 times,
  before `ErrUnsatisfiableUniqueness` names the field.
- Embedded structs (`gorm:"embedded"`) generated as columns on the owner,
  never as their own entity. Polymorphic associations are read and skipped
  by name in the `Explain` report — generation is a v2 item.
- Property-based tests (`pgregory.net/rapid`): the graph/cycle engine for any
  generated model and seed, and `gormseed.Seed` itself against real
  PostgreSQL for a linear chain, a diamond of two required principals, a
  nullable self-reference, a composite-primary-key junction and a
  shared-primary-key one-to-one — no orphaned foreign keys and no primary
  key collisions, ever.
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
- Composite unique constraints and non-auto-increment (e.g. UUID) primary
  keys are not generated — primary keys are assumed database-generated.
- The property tests and the "MegaMart" model run against PostgreSQL only;
  MySQL's own coverage is a required-FK chain and its `LastInsertId` batch
  arithmetic, not the same depth. SQLite runs `Seed`/`Explain` in the
  package's own `Example` tests and the runnable example, narrower still.
- `entseed` (ent adapter), a CLI, and every value-realism knob beyond the
  built-in rules (locale, null rate, dirty data, temporal clustering) are
  not built — see [ARCHITECTURE.md](ARCHITECTURE.md#roadmap).
