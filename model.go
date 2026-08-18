// Package autoseed reads an ORM model through an adapter's ModelSource and
// seeds a database from it: full referential integrity, deterministic
// output, no hand-written ordering or fixtures.
package autoseed

import "reflect"

// Entity describes one seedable type read from an ORM model: its name, the
// fields that hold values, and the foreign key references to other entities.
// UniqueConstraints holds one entry per composite unique index: two or more
// field names that, taken together as a tuple, must be unique across every
// row. A single-column unique constraint is expressed on the Field itself
// instead — an entry here always has two or more Fields.
type Entity struct {
	Name              string
	Fields            []Field
	References        []Reference
	UniqueConstraints [][]string
}

// Field describes one column-backed value on an Entity. Type is the Go
// type value generation must produce; Size is the column's maximum length
// for a string type, zero when the column has no declared limit. Nullable
// reports whether generation may leave this field's value out entirely
// and have persistence turn that into a real database NULL — not merely
// whether the underlying column's schema allows NULL. A column that
// allows NULL but whose Go representation has no way to express absence
// (a plain, non-pointer field with no database/sql Null* wrapper) is
// Nullable=false: generation would have nothing meaningful to leave out,
// only its own zero value.
type Field struct {
	Name          string
	Type          reflect.Type
	Size          int
	Nullable      bool
	Unique        bool
	PrimaryKey    bool
	AutoIncrement bool
	SoftDelete    bool
}

// Reference describes a foreign key on an Entity that points at another
// entity's primary key. Fields are the field names on the owning entity
// that carry the key, more than one for a composite foreign key, always in
// the same order as the referenced entity's own key fields. Target is the
// referenced Entity's Name.
type Reference struct {
	Fields   []string
	Target   string
	Nullable bool
}

// ModelSource is the contract every ORM adapter implements: reading an ORM's
// model and exposing it as entities, fields and references. Everything
// downstream of this package consumes only what Entities returns.
type ModelSource interface {
	Entities() ([]Entity, error)
}
