package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Tag has a many-to-many edge to Product, the inverse of Product's tags edge.
type Tag struct {
	ent.Schema
}

// Fields returns Tag's fields.
func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Unique(),
	}
}

// Edges returns Tag's edges.
func (Tag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("products", Product.Type).Ref("tags"),
	}
}
