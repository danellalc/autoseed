package entseed

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	entlib "entgo.io/ent"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

// ErrNilClient is returned by Seed when client is nil. Unlike Explain,
// which only reads the schema source, Seed writes rows through it.
var ErrNilClient = errors.New("entseed: client is nil")

type insertedEntity struct {
	instances []reflect.Value
}

// Seed generates and writes referentially valid rows through client, one
// entity type at a time in dependency order: required foreign keys are
// copied from an already-inserted parent row's real primary key before a
// dependent's own rows are written, and a cycle broken by a nullable
// reference is patched in a second pass once every row exists.
//
// client is the generated *ent.Client (or a type embedding one) for the
// schema at schemaPath; it must not be nil. Every entity ent's schema
// package declares is written — unlike gormseed, there is no explicit
// model list, since the generated client already names every entity as
// one of its own fields.
func Seed(ctx context.Context, client any, schemaPath string, opts ...autoseed.Option) error {
	clientValue := reflect.ValueOf(client)
	if client == nil || (clientValue.Kind() == reflect.Pointer && clientValue.IsNil()) {
		return ErrNilClient
	}

	entities, _, err := read(schemaPath)
	if err != nil {
		return err
	}

	graph, err := autoseed.NewDependencyGraph(entities)
	if err != nil {
		return err
	}
	resolved, err := graph.Resolve()
	if err != nil {
		return err
	}

	options := autoseed.NewOptions(opts...)
	root := autoseed.NewSeededSource(options.Seed)
	plan, err := autoseed.PlanGeneration(entities, resolved.Order, resolved.Deferred, root, options)
	if err != nil {
		return err
	}

	byName := entitiesByName(entities)
	planByName := make(map[string]autoseed.EntityGenerationPlan, len(plan.Entities))
	for _, p := range plan.Entities {
		planByName[p.Entity] = p
	}
	deferredByEntity := make(map[string][]autoseed.DeferredReference, len(resolved.Deferred))
	for _, d := range resolved.Deferred {
		deferredByEntity[d.Entity] = append(deferredByEntity[d.Entity], d)
	}

	generator := inference.NewDefaultGenerator()
	inserted := make(map[string]*insertedEntity, len(entities))

	for _, name := range resolved.Order {
		entity := byName[name]
		entityPlan := planByName[name]

		rows, err := generateRows(entity, entityPlan.RowCount, root.Entity(name), generator)
		if err != nil {
			return err
		}
		if err := autoseed.EnsureUnique(entity, rows, root.Entity(name)); err != nil {
			return err
		}

		instances, err := createRows(ctx, clientValue, entity, rows, entityPlan, deferredByEntity[name], inserted)
		if err != nil {
			return fmt.Errorf("entseed: creating %s: %w", name, err)
		}

		inserted[name] = &insertedEntity{instances: instances}
	}

	return assignDeferred(ctx, clientValue, resolved.Deferred, inserted)
}

func entitiesByName(entities []autoseed.Entity) map[string]autoseed.Entity {
	byName := make(map[string]autoseed.Entity, len(entities))
	for _, e := range entities {
		byName[e.Name] = e
	}
	return byName
}

func generateRows(entity autoseed.Entity, rowCount int, entitySeed *autoseed.SeededSource, generator *inference.Generator) ([]map[string]any, error) {
	rows := make([]map[string]any, rowCount)
	for i := range rows {
		row, err := generator.GenerateRow(entity, entitySeed.Row(i))
		if err != nil {
			return nil, err
		}
		rows[i] = row
	}
	return rows, nil
}

func createRows(
	ctx context.Context,
	clientValue reflect.Value,
	entity autoseed.Entity,
	rows []map[string]any,
	plan autoseed.EntityGenerationPlan,
	deferred []autoseed.DeferredReference,
	inserted map[string]*insertedEntity,
) ([]reflect.Value, error) {
	subClient := clientValue.Elem().FieldByName(entity.Name)
	if !subClient.IsValid() {
		return nil, fmt.Errorf("client has no %s field — was it built from the same schema at read time?", entity.Name)
	}
	createMethod := subClient.MethodByName("Create")
	if !createMethod.IsValid() {
		return nil, fmt.Errorf("%s has no Create method", entity.Name)
	}

	deferredFields := referenceFieldSet(deferred)
	driverRowOf := expandDriverRows(plan)
	blockLocalOf := blockLocalIndices(plan)
	driverKey := strings.Join(plan.DriverFields, "+")
	junctionKey := strings.Join(plan.JunctionFields, "+")

	instances := make([]reflect.Value, len(rows))
	for i, values := range rows {
		builder := createMethod.Call(nil)[0]
		mutationValue, mutation, err := builderMutation(builder)
		if err != nil {
			return nil, err
		}

		for fieldName, value := range values {
			if err := mutation.SetField(fieldName, value); err != nil {
				return nil, fmt.Errorf("setting %s.%s: %w", entity.Name, fieldName, err)
			}
		}

		for _, ref := range entity.References {
			refKey := strings.Join(ref.Fields, "+")
			if deferredFields[refKey] {
				continue
			}
			parent, ok := inserted[ref.Target]
			if !ok || len(parent.instances) == 0 {
				continue
			}

			parentIndex := i % len(parent.instances)
			switch refKey {
			case driverKey:
				parentIndex = driverRowOf[i]
			case junctionKey:
				n := len(parent.instances)
				if ref.Target == plan.Driver {
					parentIndex = (blockLocalOf[i] + driverRowOf[i] + 1) % n
				} else {
					parentIndex = blockLocalOf[i] % n
				}
			}

			if len(ref.Fields) != 1 {
				return nil, fmt.Errorf("entseed: %s has a composite foreign key, not supported (ent has no composite primary keys, so this shouldn't be reachable)", entity.Name)
			}
			parentID, err := entityID(parent.instances[parentIndex])
			if err != nil {
				return nil, err
			}
			if err := setEdgeParent(mutationValue, ref.Fields[0], parentID); err != nil {
				return nil, fmt.Errorf("setting %s's %s edge from parent key: %w", entity.Name, ref.Fields[0], err)
			}
		}

		saved, err := save(ctx, builder)
		if err != nil {
			return nil, err
		}
		instances[i] = saved
	}
	return instances, nil
}

func assignDeferred(ctx context.Context, clientValue reflect.Value, deferred []autoseed.DeferredReference, inserted map[string]*insertedEntity) error {
	for _, d := range deferred {
		entity, ok := inserted[d.Entity]
		if !ok {
			continue
		}
		parent, ok := inserted[d.Target]
		if !ok || len(parent.instances) == 0 {
			continue
		}
		if len(d.Fields) != 1 {
			return fmt.Errorf("entseed: %s has a composite deferred foreign key, not supported", d.Entity)
		}
		selfReference := d.Entity == d.Target

		subClient := clientValue.Elem().FieldByName(d.Entity)
		if !subClient.IsValid() {
			return fmt.Errorf("entseed: client has no %s field", d.Entity)
		}
		updateOneMethod := subClient.MethodByName("UpdateOne")
		if !updateOneMethod.IsValid() {
			return fmt.Errorf("entseed: %s has no UpdateOne method", d.Entity)
		}

		for i, instance := range entity.instances {
			parentIndex := i % len(parent.instances)
			if selfReference && parentIndex == i {
				if len(parent.instances) == 1 {
					continue
				}
				parentIndex = (parentIndex + 1) % len(parent.instances)
			}

			parentID, err := entityID(parent.instances[parentIndex])
			if err != nil {
				return err
			}

			builder := updateOneMethod.Call([]reflect.Value{instance})[0]
			mutationValue, _, err := builderMutation(builder)
			if err != nil {
				return err
			}
			if err := setEdgeParent(mutationValue, d.Fields[0], parentID); err != nil {
				return fmt.Errorf("entseed: updating deferred %s's %s edge: %w", d.Entity, d.Fields[0], err)
			}
			if _, err := save(ctx, builder); err != nil {
				return fmt.Errorf("entseed: updating deferred %s's %s edge: %w", d.Entity, d.Fields[0], err)
			}
		}
	}
	return nil
}

// builderMutation returns a create/update builder's mutation two ways:
// the reflect.Value, for calling an edge's own Set<Name>ID method (no
// generic interface reaches those), and the ent.Mutation interface, for
// SetField on regular schema fields.
func builderMutation(builder reflect.Value) (reflect.Value, entlib.Mutation, error) {
	mutationMethod := builder.MethodByName("Mutation")
	if !mutationMethod.IsValid() {
		return reflect.Value{}, nil, fmt.Errorf("entseed: builder %s has no Mutation method", builder.Type())
	}
	mutationValue := mutationMethod.Call(nil)[0]
	mutation, ok := mutationValue.Interface().(entlib.Mutation)
	if !ok {
		return reflect.Value{}, nil, fmt.Errorf("entseed: %s's mutation does not implement ent.Mutation", builder.Type())
	}
	return mutationValue, mutation, nil
}

// setEdgeParent sets a to-one edge's target by ID, via the generated
// Set<edgeName>ID(id) method reflection is the only way to reach: ent's
// generic ent.Mutation.SetField interface only covers regular schema
// fields, never an edge's foreign key.
func setEdgeParent(mutationValue reflect.Value, edgeName string, id any) error {
	setter := mutationValue.MethodByName("Set" + edgeName + "ID")
	if !setter.IsValid() {
		return fmt.Errorf("entseed: mutation %s has no Set%sID method", mutationValue.Type(), edgeName)
	}
	idValue := reflect.ValueOf(id)
	paramType := setter.Type().In(0)
	if !idValue.Type().AssignableTo(paramType) {
		if !idValue.Type().ConvertibleTo(paramType) {
			return fmt.Errorf("entseed: cannot use %s parent key of type %s as %s", edgeName, idValue.Type(), paramType)
		}
		idValue = idValue.Convert(paramType)
	}
	setter.Call([]reflect.Value{idValue})
	return nil
}

func save(ctx context.Context, builder reflect.Value) (reflect.Value, error) {
	saveMethod := builder.MethodByName("Save")
	if !saveMethod.IsValid() {
		return reflect.Value{}, fmt.Errorf("entseed: builder %s has no Save method", builder.Type())
	}
	result := saveMethod.Call([]reflect.Value{reflect.ValueOf(ctx)})
	if err, ok := result[1].Interface().(error); ok && err != nil {
		return reflect.Value{}, err
	}
	return result[0], nil
}

func entityID(instance reflect.Value) (any, error) {
	idField := instance.Elem().FieldByName("ID")
	if !idField.IsValid() {
		return nil, fmt.Errorf("entseed: %s has no ID field", instance.Type())
	}
	return idField.Interface(), nil
}

func referenceFieldSet(refs []autoseed.DeferredReference) map[string]bool {
	set := make(map[string]bool, len(refs))
	for _, ref := range refs {
		set[strings.Join(ref.Fields, "+")] = true
	}
	return set
}

func expandDriverRows(plan autoseed.EntityGenerationPlan) []int {
	if plan.Driver == "" {
		return nil
	}
	rows := make([]int, 0, plan.RowCount)
	for driverRow, count := range plan.ChildCounts {
		for i := 0; i < count; i++ {
			rows = append(rows, driverRow)
		}
	}
	return rows
}

func blockLocalIndices(plan autoseed.EntityGenerationPlan) []int {
	if plan.Driver == "" {
		return nil
	}
	indices := make([]int, 0, plan.RowCount)
	for _, count := range plan.ChildCounts {
		for i := 0; i < count; i++ {
			indices = append(indices, i)
		}
	}
	return indices
}
