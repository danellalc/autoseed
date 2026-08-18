// Package schema is a small, real ent schema used only to generate a
// client entseed's own tests exercise the adapter against.
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Customer has a required, one-directional HasMany to Order.
type Customer struct {
	ent.Schema
}

// Fields returns Customer's fields.
func (Customer) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").Unique().MaxLen(255),
		field.String("first_name"),
		field.String("last_name"),
		field.String("city"),
		field.String("street"),
		field.String("phone"),
	}
}

// Edges returns Customer's edges.
func (Customer) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("orders", Order.Type),
	}
}
