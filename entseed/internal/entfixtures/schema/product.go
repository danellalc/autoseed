package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Product has a many-to-many edge to Tag, read but not generated yet.
type Product struct {
	ent.Schema
}

// Fields returns Product's fields.
func (Product) Fields() []ent.Field {
	return []ent.Field{
		field.String("sku").Unique(),
	}
}

// Edges returns Product's edges.
func (Product) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tags", Tag.Type),
	}
}
