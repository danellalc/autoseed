package gormseed

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const insertBatchSize = 500

type insertedEntity struct {
	schema    *schema.Schema
	instances []any
}

// Seed generates and writes referentially valid rows for models through
// db, one entity type at a time in dependency order: required foreign
// keys are copied from an already-inserted parent row's real primary key
// before a dependent's own rows are written, and a cycle broken by a
// nullable reference is patched in a second pass once every row exists.
//
// models is the exhaustive set of GORM structs to seed; see Explain's doc
// for why the list is explicit.
func Seed(ctx context.Context, db *gorm.DB, models []any, opts ...autoseed.Option) error {
	entities, schemas, _, err := read(db, models)
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
	plan := autoseed.PlanGeneration(entities, resolved.Order, resolved.Deferred, root, options)

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
		entitySchema := schemas[name]

		rows, err := generateRows(entity, entityPlan.RowCount, root.Entity(name), generator)
		if err != nil {
			return err
		}
		if err := autoseed.EnsureUnique(entity, rows, root.Entity(name)); err != nil {
			return err
		}

		instances, err := buildInstances(ctx, entitySchema, entity, rows, entityPlan, deferredByEntity[name], inserted)
		if err != nil {
			return err
		}

		if err := insertBatch(db, entitySchema, instances); err != nil {
			return fmt.Errorf("gormseed: inserting %s: %w", name, err)
		}

		inserted[name] = &insertedEntity{schema: entitySchema, instances: instances}
	}

	return assignDeferred(ctx, db, resolved.Deferred, inserted)
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

func buildInstances(
	ctx context.Context,
	entitySchema *schema.Schema,
	entity autoseed.Entity,
	rows []map[string]any,
	plan autoseed.EntityGenerationPlan,
	deferred []autoseed.DeferredReference,
	inserted map[string]*insertedEntity,
) ([]any, error) {
	deferredFields := referenceFieldSet(deferred)
	driverRowOf := expandDriverRows(plan)

	instances := make([]any, len(rows))
	for i, values := range rows {
		instance := reflect.New(entitySchema.ModelType).Elem()

		for fieldName, value := range values {
			field := entitySchema.LookUpField(fieldName)
			if field == nil {
				continue
			}
			if err := field.Set(ctx, instance, value); err != nil {
				return nil, fmt.Errorf("gormseed: setting %s.%s: %w", entity.Name, fieldName, err)
			}
		}

		for _, ref := range entity.References {
			if deferredFields[strings.Join(ref.Fields, "+")] {
				continue
			}
			parent, ok := inserted[ref.Target]
			if !ok || len(parent.instances) == 0 {
				continue
			}

			parentIndex := i % len(parent.instances)
			if ref.Target == plan.Driver {
				parentIndex = driverRowOf[i]
			}
			if err := copyKey(ctx, parent.schema, reflect.ValueOf(parent.instances[parentIndex]).Elem(), entitySchema, instance, ref.Fields); err != nil {
				return nil, err
			}
		}

		instances[i] = instance.Addr().Interface()
	}
	return instances, nil
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

func copyKey(ctx context.Context, parentSchema *schema.Schema, parentInstance reflect.Value, childSchema *schema.Schema, childInstance reflect.Value, childFields []string) error {
	for i, childFieldName := range childFields {
		if i >= len(parentSchema.PrimaryFields) {
			return fmt.Errorf("gormseed: %s has more foreign key fields than %s has primary key fields", childSchema.Name, parentSchema.Name)
		}
		value, zero := parentSchema.PrimaryFields[i].ValueOf(ctx, parentInstance)
		if zero {
			continue
		}
		childField := childSchema.LookUpField(childFieldName)
		if err := childField.Set(ctx, childInstance, value); err != nil {
			return fmt.Errorf("gormseed: setting %s.%s from parent key: %w", childSchema.Name, childFieldName, err)
		}
	}
	return nil
}

func insertBatch(db *gorm.DB, entitySchema *schema.Schema, instances []any) error {
	if len(instances) == 0 {
		return nil
	}

	sliceType := reflect.SliceOf(reflect.PointerTo(entitySchema.ModelType))
	slice := reflect.MakeSlice(sliceType, len(instances), len(instances))
	for i, instance := range instances {
		slice.Index(i).Set(reflect.ValueOf(instance))
	}
	slicePtr := reflect.New(sliceType)
	slicePtr.Elem().Set(slice)

	return db.Table(entitySchema.Table).CreateInBatches(slicePtr.Interface(), batchSizeFor(entitySchema)).Error
}

func batchSizeFor(entitySchema *schema.Schema) int {
	for _, field := range entitySchema.Fields {
		if field.DBName != "" && !field.PrimaryKey {
			return insertBatchSize
		}
	}
	return 1
}

func assignDeferred(ctx context.Context, db *gorm.DB, deferred []autoseed.DeferredReference, inserted map[string]*insertedEntity) error {
	if len(deferred) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, d := range deferred {
			entity, ok := inserted[d.Entity]
			if !ok {
				continue
			}
			parent, ok := inserted[d.Target]
			if !ok || len(parent.instances) == 0 {
				continue
			}
			selfReference := d.Entity == d.Target

			for i, instance := range entity.instances {
				parentIndex := i % len(parent.instances)
				if selfReference && parentIndex == i {
					if len(parent.instances) == 1 {
						continue
					}
					parentIndex = (parentIndex + 1) % len(parent.instances)
				}

				parentValue := reflect.ValueOf(parent.instances[parentIndex]).Elem()
				instanceValue := reflect.ValueOf(instance).Elem()
				if err := copyKey(ctx, parent.schema, parentValue, entity.schema, instanceValue, d.Fields); err != nil {
					return err
				}
				if err := tx.Table(entity.schema.Table).Save(instance).Error; err != nil {
					return fmt.Errorf("gormseed: updating deferred %s.%s: %w", d.Entity, strings.Join(d.Fields, "+"), err)
				}
			}
		}
		return nil
	})
}
