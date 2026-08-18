package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Order has a required belongs-to Customer, the inverse of Customer's orders edge.
type Order struct {
	ent.Schema
}

// Fields returns Order's fields.
func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.String("street"),
		field.String("city"),
		field.String("notes").Optional(),
	}
}

// Edges returns Order's edges.
func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("customer", Customer.Type).Ref("orders").Unique().Required(),
	}
}
