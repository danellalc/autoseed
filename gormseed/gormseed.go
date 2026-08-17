// Package gormseed reads a GORM model through schema.Parse and adapts it
// to the autoseed.ModelSource contract.
package gormseed

import (
	"github.com/danellalc/autoseed"
	"gorm.io/gorm"
)

type source struct {
	entities []autoseed.Entity
}

func (s source) Entities() ([]autoseed.Entity, error) {
	return s.entities, nil
}

// Explain reads models through db's GORM schema and resolves the result
// into a Plan, without writing anything.
//
// models is the exhaustive set of GORM model structs the caller wants
// seeded. A *gorm.DB keeps no registry of every struct it has ever used —
// unlike an EF Core DbContext, it cannot answer "what models do you know
// about" — so the list is the one thing the caller must state explicitly.
// db may be nil, or not yet connected to a database: reading the model
// only inspects the Go struct types.
func Explain(db *gorm.DB, models []any) (*autoseed.Plan, error) {
	entities, _, skipped, err := read(db, models)
	if err != nil {
		return nil, err
	}
	return autoseed.Explain(source{entities: entities}, skipped...)
}
