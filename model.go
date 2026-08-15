// Package autoseed reads an ORM model through an adapter's ModelSource and
// seeds a database from it: full referential integrity, deterministic
// output, no hand-written ordering or fixtures.
package autoseed

// Entity describes one seedable type read from an ORM model: its name, the
// fields that hold values, and the foreign key references to other entities.
type Entity struct {
	Name       string
	Fields     []Field
	References []Reference
}

// Field describes one column-backed value on an Entity.
type Field struct {
	Name     string
	Nullable bool
	Unique   bool
}

// Reference describes a foreign key on an Entity that points at another
// entity's primary key. Name is the field on the owning entity that carries
// the key; Target is the referenced Entity's Name.
type Reference struct {
	Name     string
	Target   string
	Nullable bool
}

// ModelSource is the contract every ORM adapter implements: reading an ORM's
// model and exposing it as entities, fields and references. Everything
// downstream of this package consumes only what Entities returns.
type ModelSource interface {
	Entities() ([]Entity, error)
}
