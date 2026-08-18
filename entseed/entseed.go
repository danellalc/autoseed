// Package entseed reads an ent schema via entc.LoadGraph and adapts it to
// the autoseed.ModelSource contract.
//
// Unlike GORM, ent exposes its resolved schema graph only through
// entc.LoadGraph, which type-checks and loads the schema package from
// source — the same mechanism entc itself uses during `go generate`, not
// a runtime introspection of a live *ent.Client the way schema.Parse
// works on a *gorm.DB. Every function here takes schemaPath, the Go
// import path (or relative path) to the package holding the ent.Schema
// definitions (typically "./ent/schema"), rather than a client or model
// list.
package entseed

import (
	"github.com/danellalc/autoseed"
)

type source struct {
	entities []autoseed.Entity
}

func (s source) Entities() ([]autoseed.Entity, error) {
	return s.entities, nil
}

// Explain reads the ent schema at schemaPath and resolves the result
// into a Plan, without writing anything.
func Explain(schemaPath string) (*autoseed.Plan, error) {
	entities, _, err := read(schemaPath)
	if err != nil {
		return nil, err
	}
	return autoseed.Explain(source{entities: entities})
}
