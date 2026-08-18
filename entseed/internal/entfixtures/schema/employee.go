package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

// Employee has a nullable, self-referencing manager/reports edge pair.
type Employee struct {
	ent.Schema
}

// Edges returns Employee's edges.
func (Employee) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("reports", Employee.Type).From("manager").Unique(),
	}
}
