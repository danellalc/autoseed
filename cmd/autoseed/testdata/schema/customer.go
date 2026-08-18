// Package schema is a minimal ent schema used only to test the autoseed
// CLI's explain command against a real schema path -- Explain reads
// schema source directly, so no generated client is needed here.
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
		field.String("email").Unique(),
	}
}

// Edges returns Customer's edges.
func (Customer) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("orders", Order.Type),
	}
}
