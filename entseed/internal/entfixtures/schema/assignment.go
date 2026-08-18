package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/index"
)

// Assignment has three required edges -- Student, Course, Teacher --
// unique only together: a ternary attributed join via a composite unique
// index over all three edges.
type Assignment struct {
	ent.Schema
}

// Edges returns Assignment's edges.
func (Assignment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("student", Student.Type).Ref("assignments").Unique().Required(),
		edge.From("course", Course.Type).Ref("assignments").Unique().Required(),
		edge.From("teacher", Teacher.Type).Ref("assignments").Unique().Required(),
	}
}

// Indexes returns Assignment's indexes.
func (Assignment) Indexes() []ent.Index {
	return []ent.Index{
		index.Edges("student", "course", "teacher").Unique(),
	}
}
