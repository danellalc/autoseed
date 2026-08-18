package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

// Course has many Assignments via a required, one-directional edge.
type Course struct {
	ent.Schema
}

// Edges returns Course's edges.
func (Course) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("assignments", Assignment.Type),
	}
}
