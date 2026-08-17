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
  cycles patched in a second, deferred pass.
- Deterministic generation: every random draw derives from one root seed,
  hierarchically and positionally (root → entity → row index → field). Same
  seed, same data, always — verified by a dedicated test, including an
  anti-map-iteration guard.
- Stable topological sort (ties break on entity name) and Tarjan-based cycle
  detection; a required cycle with no nullable edge to break it returns
  `ErrUnsatisfiableCycle`, naming the real cycle path.
- `autoseed.GenerationPlan`: long-tail cardinality (Exponential distribution)
  for related row counts, one driving principal per dependent entity, a
  required non-driver reference target is never left at zero rows.
- ~16 built-in `inference` rules over gofakeit v7 (seeded): name/email/phone/
  address/URL, coherent `CreatedAt`/`UpdatedAt` and `Price`×`Quantity`=`Total`
  on the same row, a hand-rolled Brazilian CPF/CNPJ check-digit generator (no
  locale mechanism yet — matches on field name alone), and a soft-delete rule
  that leaves `gorm.DeletedAt` unset on 90% of rows.
- `autoseed.EnsureUnique`: duplicate values for a single-column string unique
  field are rewritten with a random numeric suffix, retried up to 20 times,
  before `ErrUnsatisfiableUniqueness` names the field.
- Embedded structs (`gorm:"embedded"`) generated as columns on the owner,
  never as their own entity. Polymorphic associations are read and skipped
  by name in the `Explain` report — generation is a v2 item.
- Property-based tests (`pgregory.net/rapid`): the graph/cycle engine for any
  generated model and seed, and `gormseed.Seed` itself against real
  PostgreSQL for a linear chain, a diamond of two required principals and a
  nullable self-reference — no orphaned foreign keys, ever.
- `examples/gormseed-basic`, a runnable example (its own Go module, `replace`
  points at this checkout) seeding a real SQLite database end to end.

### Known gaps

- `gormseed.SeedCoverage` (the smallest dataset that exercises everything) is
  not built.
- Composite unique constraints and non-auto-increment (e.g. UUID) primary
  keys are not generated — primary keys are assumed database-generated.
- A true one-to-one relationship (a child whose primary key is also its only
  foreign key) is not modeled distinctly from one-to-many: cardinality is
  always a long-tail draw.
- SQLite is a supported target but untested against real `Seed` runs (the
  test suite intentionally never treats it as the only surface).
- `entseed` (ent adapter), a CLI, and every value-realism knob beyond the
  built-in rules (locale, null rate, dirty data, temporal clustering) are
  not built — see [ARCHITECTURE.md](ARCHITECTURE.md#roadmap).
