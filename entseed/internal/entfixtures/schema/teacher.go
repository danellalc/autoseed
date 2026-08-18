package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
)

// Teacher has many Assignments via a required, one-directional edge.
type Teacher struct {
	ent.Schema
}

// Edges returns Teacher's edges.
func (Teacher) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("assignments", Assignment.Type),
	}
}
