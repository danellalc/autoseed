// Package autoseed reads an ORM model through an adapter's ModelSource and
// seeds a database from it: full referential integrity, deterministic
// output, no hand-written ordering or fixtures.
package autoseed

import "reflect"

// Entity describes one seedable type read from an ORM model: its name, the
// fields that hold values, and the foreign key references to other entities.
type Entity struct {
	Name       string
	Fields     []Field
	References []Reference
}

// Field describes one column-backed value on an Entity. Type is the Go
// type value generation must produce; Size is the column's maximum length
// for a string type, zero when the column has no declared limit.
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
