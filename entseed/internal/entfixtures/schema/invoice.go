package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Invoice has a composite unique index on Series and Number together --
// neither field is unique on its own.
type Invoice struct {
	ent.Schema
}

// Fields returns Invoice's fields.
func (Invoice) Fields() []ent.Field {
	return []ent.Field{
		field.String("series"),
		// StorageKey deliberately differs from the field's own ent name,
		// so uniqueConstraints' column-to-name resolution is exercised
		// against a real mismatch, not just the common case where the two
		// happen to be equal.
		field.String("number").StorageKey("invoice_number_column"),
	}
}

// Indexes returns Invoice's indexes.
func (Invoice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("series", "number").Unique(),
	}
}
