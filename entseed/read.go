package entseed

import (
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	entfield "entgo.io/ent/schema/field"
	"github.com/google/uuid"

	"github.com/danellalc/autoseed"
)

func read(schemaPath string) ([]autoseed.Entity, *gen.Graph, error) {
	graph, err := entc.LoadGraph(schemaPath, &gen.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("entseed: loading schema %q: %w", schemaPath, err)
	}

	entities := make([]autoseed.Entity, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		entity, err := buildEntity(node)
		if err != nil {
			return nil, nil, err
		}
		entities = append(entities, entity)
	}
	return entities, graph, nil
}

func buildEntity(node *gen.Type) (autoseed.Entity, error) {
	idType, err := goType(node.ID.Type)
	if err != nil {
		return autoseed.Entity{}, fmt.Errorf("entseed: %s.%s: %w", node.Name, node.ID.Name, err)
	}
	fields := []autoseed.Field{{
		Name:          node.ID.Name,
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
			return autoseed.Entity{}, fmt.Errorf("entseed: %s.%s: %w", node.Name, f.Name, err)
		}
		fields = append(fields, autoseed.Field{
			Name:     f.Name,
			Type:     fieldType,
			Nullable: f.Optional,
			Unique:   f.Unique,
		})
	}

	var references []autoseed.Reference
	for _, e := range node.Edges {
		if !e.OwnFK() || e.M2M() {
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

	return autoseed.Entity{Name: node.Name, Fields: fields, References: references}, nil
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
