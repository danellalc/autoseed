package gormseed

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const insertBatchSize = 500

// ErrNilDB is returned by Seed when db is nil. Unlike Explain, which only
// inspects Go struct types, Seed writes rows and needs a real, migrated
// connection to write them through.
var ErrNilDB = errors.New("gormseed: db is nil")

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
// for why the list is explicit. db must not be nil.
func Seed(ctx context.Context, db *gorm.DB, models []any, opts ...autoseed.Option) error {
	if db == nil {
		return ErrNilDB
	}

	entities, schemas, resolved, err := resolveModel(db, models)
	if err != nil {
		return err
	}

	options := autoseed.NewOptions(opts...)
	root := autoseed.NewSeededSource(options.Seed)
	plan, err := autoseed.PlanGeneration(entities, resolved.Order, resolved.Deferred, root, options)
	if err != nil {
		return err
	}

	return seedPlan(ctx, db, entities, schemas, resolved, plan, options, root, false)
}

// SeedCoverage writes the smallest dataset that exercises every field
// and relationship shape the model has — every Nullable field both left
// out and given a value, every bool-kind field true and false, every
// sized string field at empty, one character and its declared maximum
// length, every relationship at zero, one and several children —
// instead of a large, realistic bulk dataset. Usually well under 50
// rows.
//
// WithScale and WithNilRate are ignored: row counts come from the
// model's own shape, not a scale factor, and every Nullable field's
// null-vs-not split is decided by coverage itself, not a probability.
// WithSeed and WithLocale still apply, to whatever value a field gets on
// the rows coverage doesn't specifically constrain.
func SeedCoverage(ctx context.Context, db *gorm.DB, models []any, opts ...autoseed.Option) error {
	if db == nil {
		return ErrNilDB
	}

	entities, schemas, resolved, err := resolveModel(db, models)
	if err != nil {
		return err
	}

	plan, err := autoseed.PlanCoverage(entities, resolved.Order, resolved.Deferred)
	if err != nil {
		return err
	}

	options := autoseed.NewOptions(opts...)
	root := autoseed.NewSeededSource(options.Seed)
	return seedPlan(ctx, db, entities, schemas, resolved, plan, options, root, true)
}

func resolveModel(db *gorm.DB, models []any) ([]autoseed.Entity, map[string]*schema.Schema, *autoseed.TopologicalSortResult, error) {
	entities, schemas, _, err := read(db, models)
	if err != nil {
		return nil, nil, nil, err
	}

	graph, err := autoseed.NewDependencyGraph(entities)
	if err != nil {
		return nil, nil, nil, err
	}
	resolved, err := graph.Resolve()
	if err != nil {
		return nil, nil, nil, err
	}
	return entities, schemas, resolved, nil
}

// seedPlan writes rows for a plan PlanGeneration or PlanCoverage already
// computed; only the plan itself, whether coverage's own field-axis
// overrides apply before EnsureUnique runs, and whether options.NilRate
// reaches the generator at all, differ between Seed and SeedCoverage.
func seedPlan(
	ctx context.Context,
	db *gorm.DB,
	entities []autoseed.Entity,
	schemas map[string]*schema.Schema,
	resolved *autoseed.TopologicalSortResult,
	plan *autoseed.GenerationPlan,
	options autoseed.Options,
	root *autoseed.SeededSource,
	coverage bool,
) error {
	byName := entitiesByName(entities)
	planByName := make(map[string]autoseed.EntityGenerationPlan, len(plan.Entities))
	for _, p := range plan.Entities {
		planByName[p.Entity] = p
	}
	deferredByEntity := make(map[string][]autoseed.DeferredReference, len(resolved.Deferred))
	for _, d := range resolved.Deferred {
		deferredByEntity[d.Entity] = append(deferredByEntity[d.Entity], d)
	}

	nilRate := options.NilRate
	if coverage {
		// GenerateRow's own probabilistic nil roll must stay out of
		// coverage's way: ApplyCoverageOverrides only force-nils the even
		// rows of a Nullable field, trusting the odd rows to land on a
		// real, non-nil value — a leftover NilRate>0 from a caller who
		// passed WithNilRate to SeedCoverage anyway could null one of
		// those odd rows too, silently defeating the "both states appear"
		// guarantee the godoc promises.
		nilRate = 0
	}
	generator := inference.NewDefaultGenerator().WithNilRate(nilRate).WithLocale(options.Locale)
	inserted := make(map[string]*insertedEntity, len(entities))

	for _, name := range resolved.Order {
		entity := byName[name]
		entityPlan := planByName[name]
		entitySchema := schemas[name]

		rows, err := generateRows(entity, entityPlan.RowCount, root.Entity(name), generator)
		if err != nil {
			return err
		}
		if coverage {
			autoseed.ApplyCoverageOverrides(entity, rows)
		}
		if err := autoseed.EnsureUnique(entity, rows, root.Entity(name)); err != nil {
			return err
		}

		instances, err := buildInstances(ctx, entitySchema, entity, rows, entityPlan, deferredByEntity[name], inserted)
		if err != nil {
			return err
		}

		if err := insertBatch(ctx, db, entitySchema, instances); err != nil {
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
	blockLocalOf := blockLocalIndices(plan)
	driverKey := strings.Join(plan.DriverFields, "+")
	junctionPositionOf := junctionPositions(plan)
	rowsOf := func(target string) int {
		if parent, ok := inserted[target]; ok {
			return len(parent.instances)
		}
		return 0
	}

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

		var junctionIdx []int
		if len(plan.JunctionParticipants) > 0 {
			junctionIdx = autoseed.JunctionIndices(plan.JunctionParticipants, plan.Driver, driverRowOf[i], blockLocalOf[i], rowsOf)
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

			// Matched by the reference's own foreign key fields, never by
			// Target alone: a self-referencing many-to-many gives the
			// driver and junction references the same Target, and only
			// their Fields tell them apart.
			parentIndex := i % len(parent.instances)
			if refKey == driverKey {
				parentIndex = driverRowOf[i]
			} else if pos, ok := junctionPositionOf[refKey]; ok {
				parentIndex = junctionIdx[pos]
			}
			if err := copyKey(ctx, parent.schema, reflect.ValueOf(parent.instances[parentIndex]).Elem(), entitySchema, instance, ref.Fields); err != nil {
				return nil, err
			}
		}

		instances[i] = instance.Addr().Interface()
	}
	return instances, nil
}

// junctionPositions maps each participant's own Fields key to its index
// within plan.JunctionParticipants, so a reference on the entity can be
// matched to the JunctionIndices slot computed for it.
func junctionPositions(plan autoseed.EntityGenerationPlan) map[string]int {
	positions := make(map[string]int, len(plan.JunctionParticipants))
	for i, p := range plan.JunctionParticipants {
		positions[strings.Join(p.Fields, "+")] = i
	}
	return positions
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

// blockLocalIndices returns, for each row in driver-row order, its
// position within its own driver row's block of children — 0 at the
// start of every block, unlike a row's position in the full entity.
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

func insertBatch(ctx context.Context, db *gorm.DB, entitySchema *schema.Schema, instances []any) error {
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

	return db.WithContext(ctx).Table(entitySchema.Table).CreateInBatches(slicePtr.Interface(), batchSizeFor(entitySchema)).Error
}

// batchSizeFor returns the ordinary batch size unless entitySchema has no
// field GORM will actually list in the INSERT statement — every field
// either auto-increment or otherwise not creatable (gorm:"->" read-only,
// gorm:"<-:false", a DB-computed column), so GORM emits
// INSERT ... DEFAULT VALUES, which has no multi-row form and only ever
// reports the first row's generated ID back. A composite primary key made
// of non-auto-increment, creatable foreign keys (a many-to-many join
// table, an "attributed join" entity, a shared-primary-key one-to-one)
// still has real values to list and batches normally.
func batchSizeFor(entitySchema *schema.Schema) int {
	for _, field := range entitySchema.Fields {
		if field.DBName != "" && field.Creatable && !field.AutoIncrement {
			return insertBatchSize
		}
	}
	return 1
}

func assignDeferred(ctx context.Context, db *gorm.DB, deferred []autoseed.DeferredReference, inserted map[string]*insertedEntity) error {
	if len(deferred) == 0 {
		return nil
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
