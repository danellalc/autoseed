package entseed

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	entfield "entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/danellalc/autoseed"
)

func read(schemaPath string) ([]autoseed.Entity, *gen.Graph, []autoseed.SkipReason, error) {
	graph, err := entc.LoadGraph(schemaPath, &gen.Config{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("entseed: loading schema %q: %w", schemaPath, err)
	}

	entities := make([]autoseed.Entity, 0, len(graph.Nodes))
	var skipped []autoseed.SkipReason
	for _, node := range graph.Nodes {
		entity, nodeSkipped, err := buildEntity(node)
		if err != nil {
			return nil, nil, nil, err
		}
		entities = append(entities, entity)
		skipped = append(skipped, nodeSkipped...)
	}
	return entities, graph, skipped, nil
}

func buildEntity(node *gen.Type) (autoseed.Entity, []autoseed.SkipReason, error) {
	idType, err := goType(node.ID.Type)
	if err != nil {
		return autoseed.Entity{}, nil, fmt.Errorf("entseed: %s.%s: %w", node.Name, node.ID.Name, err)
	}
	fields := []autoseed.Field{{
		Name:          pascal(node.ID.Name),
		Type:          idType,
		PrimaryKey:    true,
		AutoIncrement: !node.ID.UserDefined,
	}}

	for _, f := range node.Fields {
		if f.IsEdgeField() {
			continue
		}
		fieldType, err := goType(f.Type)
		if err != nil {
			return autoseed.Entity{}, nil, fmt.Errorf("entseed: %s.%s: %w", node.Name, f.Name, err)
		}
		fields = append(fields, autoseed.Field{
			// Pascal-cased, not ent's own declared name verbatim, so a
			// field an idiomatic ent schema declares snake_case (e.g.
			// "created_at") still ends in the same "CreatedAt"-style
			// suffix inference's named rules match against -- the GORM
			// convention every rule was written for, since a Go struct
			// field name is always already PascalCase.
			Name:     pascal(f.Name),
			Type:     fieldType,
			Nullable: f.Optional,
			Unique:   f.Unique,
		})
	}

	var references []autoseed.Reference
	var skipped []autoseed.SkipReason
	for _, e := range node.Edges {
		if e.M2M() {
			// ent links a many-to-many pair through dedicated
			// Add<Edge>IDs-style edge mutation methods, never by
			// inserting into an addressable join-table entity the way
			// GORM's auto-generated join struct works -- generation for
			// this shape needs its own design, not a partial guess.
			// Reported from one side only (the non-inverse one) so a
			// symmetric M2M pair isn't named twice.
			if !e.IsInverse() {
				skipped = append(skipped, autoseed.SkipReason{
					Entity: node.Name,
					Field:  e.Name,
					Reason: "many-to-many edges are read but not generated yet",
				})
			}
			continue
		}
		if !e.OwnFK() {
			continue
		}
		// The mutation's generic SetField only reaches regular schema
		// fields, never an edge's foreign key -- that always goes through
		// a dedicated Set<Name>ID(id) method instead, so Fields holds the
		// edge's own (pascal-cased) name, not a database column name.
		references = append(references, autoseed.Reference{
			Fields:   []string{pascal(e.Name)},
			Target:   e.Type.Name,
			Nullable: e.Optional,
		})
	}

	return autoseed.Entity{
		Name:              node.Name,
		Fields:            fields,
		References:        references,
		UniqueConstraints: uniqueConstraints(node),
	}, skipped, nil
}

// storageKeyByName maps each of node's own (non-edge) field names,
// including its ID, to its real storage key. ent's generated
// Mutation.SetField matches against a field's storage key, not its
// declared name — the two coincide unless a field declares an explicit
// StorageKey override, so persist.go must translate through this map
// rather than pass a field's own Name straight to SetField. Keyed by the
// same pascal-cased name buildEntity gives the field, not ent's own
// declared name, since that's what a row's generated values map uses.
func storageKeyByName(node *gen.Type) map[string]string {
	keys := make(map[string]string, len(node.Fields)+1)
	keys[pascal(node.ID.Name)] = node.ID.StorageKey()
	for _, f := range node.Fields {
		if f.IsEdgeField() {
			continue
		}
		keys[pascal(f.Name)] = f.StorageKey()
	}
	return keys
}

// uniqueConstraints returns node's composite unique indexes -- an
// index.Fields(...).Unique() or index.Edges(...).Unique() index declared
// on the schema's Indexes method -- each as a field-name tuple in the
// index's own column order, using the same name a plain Field or an
// edge-owned Reference already carries elsewhere in this file (an edge's
// own pascal-cased name, not its underlying storage column). A
// single-column unique index is already carried on that Field's own
// Unique flag instead and is not repeated here.
func uniqueConstraints(node *gen.Type) [][]string {
	nameByColumn := make(map[string]string, len(node.Fields)+len(node.Edges))
	for _, f := range node.Fields {
		if f.IsEdgeField() {
			continue
		}
		nameByColumn[f.StorageKey()] = pascal(f.Name)
	}
	for _, e := range node.Edges {
		if e.OwnFK() {
			nameByColumn[e.Rel.Column()] = pascal(e.Name)
		}
	}

	var constraints [][]string
	for _, idx := range node.Indexes {
		if !idx.Unique || len(idx.Columns) < 2 {
			continue
		}
		fields := make([]string, 0, len(idx.Columns))
		complete := true
		for _, column := range idx.Columns {
			name, ok := nameByColumn[column]
			if !ok {
				complete = false
				break
			}
			fields = append(fields, name)
		}
		if complete {
			constraints = append(constraints, fields)
		}
	}
	sort.Slice(constraints, func(i, j int) bool {
		return strings.Join(constraints[i], "+") < strings.Join(constraints[j], "+")
	})
	return constraints
}

// pascal converts an ent identifier (camelCase or snake_case) to the
// PascalCase form ent's own codegen uses for method names, e.g. the edge
// name "customer" becomes "Customer" to match the generated
// SetCustomerID method.
func pascal(name string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range name {
		if r == '_' {
			upperNext = true
			continue
		}
		if upperNext {
			b.WriteRune(unicode.ToUpper(r))
			upperNext = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

var (
	uuidType = reflect.TypeOf(uuid.UUID{})
	timeType = reflect.TypeOf(time.Time{})
)

func goType(t *entfield.TypeInfo) (reflect.Type, error) {
	switch t.Type {
	case entfield.TypeBool:
		return reflect.TypeOf(false), nil
	case entfield.TypeTime:
		return timeType, nil
	case entfield.TypeUUID:
		return uuidType, nil
	case entfield.TypeString, entfield.TypeEnum:
		return reflect.TypeOf(""), nil
	case entfield.TypeInt8:
		return reflect.TypeOf(int8(0)), nil
	case entfield.TypeInt16:
		return reflect.TypeOf(int16(0)), nil
	case entfield.TypeInt32:
		return reflect.TypeOf(int32(0)), nil
	case entfield.TypeInt:
		return reflect.TypeOf(int(0)), nil
	case entfield.TypeInt64:
		return reflect.TypeOf(int64(0)), nil
	case entfield.TypeUint8:
		return reflect.TypeOf(uint8(0)), nil
	case entfield.TypeUint16:
		return reflect.TypeOf(uint16(0)), nil
	case entfield.TypeUint32:
		return reflect.TypeOf(uint32(0)), nil
	case entfield.TypeUint:
		return reflect.TypeOf(uint(0)), nil
	case entfield.TypeUint64:
		return reflect.TypeOf(uint64(0)), nil
	case entfield.TypeFloat32:
		return reflect.TypeOf(float32(0)), nil
	case entfield.TypeFloat64:
		return reflect.TypeOf(float64(0)), nil
	default:
		return nil, fmt.Errorf("%w: ent type %q", autoseed.ErrUnsupportedField, t.String())
	}
}
