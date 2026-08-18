package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

// Order has a required belongs-to Customer, the inverse of Customer's
// orders edge.
type Order struct {
	ent.Schema
}

// Edges returns Order's edges.
func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("customer", Customer.Type).Ref("orders").Unique().Required(),
	}
}
