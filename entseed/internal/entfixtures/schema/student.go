package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

// Student has many Assignments via a required, one-directional edge.
type Student struct {
	ent.Schema
}

// Edges returns Student's edges.
func (Student) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("assignments", Assignment.Type),
	}
}
