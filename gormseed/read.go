package gormseed

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type inferredReference struct {
	fields []string
	target string
}

func read(db *gorm.DB, models []any) ([]autoseed.Entity, map[string]*schema.Schema, []autoseed.SkipReason, error) {
	schemas, order, err := parseAll(db, models)
	if err != nil {
		return nil, nil, nil, err
	}

	inferredByChild := make(map[string][]inferredReference, len(order))
	joinTables := make(map[string]autoseed.Entity)
	var skipped []autoseed.SkipReason
	seenSkip := make(map[[2]string]bool)

	for _, name := range order {
		s := schemas[name]

		for _, r := range childOwnedRelationships(s) {
			if r.Polymorphic != nil {
				key := [2]string{r.FieldSchema.Name, r.Polymorphic.PolymorphicID.Name}
				if seenSkip[key] {
					continue
				}
				seenSkip[key] = true
				skipped = append(skipped, autoseed.SkipReason{
					Entity: r.FieldSchema.Name,
					Field:  r.Polymorphic.PolymorphicID.Name,
					Reason: "polymorphic association: generation not supported yet (v2)",
				})
				continue
			}

			fields, err := foreignKeyFields(r, r.Schema)
			if err != nil {
				return nil, nil, nil, err
			}
			inferredByChild[r.FieldSchema.Name] = append(inferredByChild[r.FieldSchema.Name], inferredReference{
				fields: fields,
				target: r.Schema.Name,
			})
		}

		for _, r := range s.Relationships.Many2Many {
			if r.JoinTable == nil {
				continue
			}
			if _, exists := joinTables[r.JoinTable.Name]; exists {
				continue
			}
			entity, err := manyToManyJoinEntity(r)
			if err != nil {
				return nil, nil, nil, err
			}
			joinTables[entity.Name] = entity
			schemas[entity.Name] = r.JoinTable
		}
	}

	entities := make([]autoseed.Entity, 0, len(order)+len(joinTables))
	for _, name := range order {
		entity, err := buildEntity(schemas[name], inferredByChild[name])
		if err != nil {
			return nil, nil, nil, err
		}
		entities = append(entities, entity)
	}

	joinNames := make([]string, 0, len(joinTables))
	for name := range joinTables {
		joinNames = append(joinNames, name)
	}
	sort.Strings(joinNames)
	for _, name := range joinNames {
		entities = append(entities, joinTables[name])
	}

	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].Entity != skipped[j].Entity {
			return skipped[i].Entity < skipped[j].Entity
		}
		return skipped[i].Field < skipped[j].Field
	})

	return entities, schemas, skipped, nil
}

func parseAll(db *gorm.DB, models []any) (map[string]*schema.Schema, []string, error) {
	parse := parser(db)

	schemas := make(map[string]*schema.Schema, len(models))
	order := make([]string, 0, len(models))

	for _, model := range models {
		s, err := parse(model)
		if err != nil {
			return nil, nil, fmt.Errorf("gormseed: parsing %T: %w", model, err)
		}
		if _, exists := schemas[s.Name]; exists {
			continue
		}
		schemas[s.Name] = s
		order = append(order, s.Name)
	}

	return schemas, order, nil
}

func parser(db *gorm.DB) func(any) (*schema.Schema, error) {
	if db == nil {
		cache := &sync.Map{}
		namer := schema.NamingStrategy{}
		return func(model any) (*schema.Schema, error) {
			return schema.Parse(model, cache, namer)
		}
	}
	return func(model any) (*schema.Schema, error) {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			return nil, err
		}
		return stmt.Schema, nil
	}
}

func childOwnedRelationships(s *schema.Schema) []*schema.Relationship {
	rels := make([]*schema.Relationship, 0, len(s.Relationships.HasMany)+len(s.Relationships.HasOne))
	rels = append(rels, s.Relationships.HasMany...)
	rels = append(rels, s.Relationships.HasOne...)
	return rels
}

func foreignKeyFields(r *schema.Relationship, target *schema.Schema) ([]string, error) {
	byPrimaryKey := make(map[string]string, len(r.References))
	for _, ref := range r.References {
		if ref.ForeignKey == nil || ref.PrimaryKey == nil {
			continue
		}
		byPrimaryKey[ref.PrimaryKey.Name] = ref.ForeignKey.Name
	}
	if len(byPrimaryKey) == 0 {
		return nil, fmt.Errorf("gormseed: relationship %q on %q has no foreign key fields", r.Name, r.Schema.Name)
	}

	fields := make([]string, 0, len(byPrimaryKey))
	for _, pk := range target.PrimaryFields {
		if fk, ok := byPrimaryKey[pk.Name]; ok {
			fields = append(fields, fk)
		}
	}
	if len(fields) != len(byPrimaryKey) {
		return nil, fmt.Errorf("gormseed: relationship %q on %q: foreign key fields do not match %q's own primary key fields", r.Name, r.Schema.Name, target.Name)
	}
	return fields, nil
}

func buildEntity(s *schema.Schema, inferred []inferredReference) (autoseed.Entity, error) {
	byName := make(map[string]*schema.Field, len(s.Fields))
	fields := make([]autoseed.Field, 0, len(s.Fields))
	for _, f := range s.Fields {
		if f.DBName == "" {
			continue
		}
		byName[f.Name] = f
		fields = append(fields, fieldFrom(f))
	}

	seen := make(map[string]bool)
	var refs []autoseed.Reference

	for _, r := range s.Relationships.BelongsTo {
		fkFields, err := foreignKeyFields(r, r.FieldSchema)
		if err != nil {
			return autoseed.Entity{}, err
		}
		ref := autoseed.Reference{
			Fields:   fkFields,
			Target:   r.FieldSchema.Name,
			Nullable: fieldsNullable(byName, fkFields),
		}
		seen[referenceKey(ref)] = true
		refs = append(refs, ref)
	}

	for _, ir := range inferred {
		ref := autoseed.Reference{
			Fields:   ir.fields,
			Target:   ir.target,
			Nullable: fieldsNullable(byName, ir.fields),
		}
		key := referenceKey(ref)
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, ref)
	}

	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Target != refs[j].Target {
			return refs[i].Target < refs[j].Target
		}
		return strings.Join(refs[i].Fields, "+") < strings.Join(refs[j].Fields, "+")
	})

	return autoseed.Entity{Name: s.Name, Fields: fields, References: refs, UniqueConstraints: uniqueConstraints(s)}, nil
}

// uniqueConstraints returns s's composite unique indexes — a
// gorm:"uniqueIndex:name" tag shared by two or more fields — each as a
// field-name tuple in the index's own column order. A single-column
// unique index is already carried on that Field's own Unique flag instead
// and is not repeated here.
func uniqueConstraints(s *schema.Schema) [][]string {
	var constraints [][]string
	for _, idx := range s.ParseIndexes() {
		if idx.Class != "UNIQUE" || len(idx.Fields) < 2 {
			continue
		}
		fields := make([]string, len(idx.Fields))
		for i, f := range idx.Fields {
			fields[i] = f.Name
		}
		constraints = append(constraints, fields)
	}
	sort.Slice(constraints, func(i, j int) bool {
		return strings.Join(constraints[i], "+") < strings.Join(constraints[j], "+")
	})
	return constraints
}

func referenceKey(ref autoseed.Reference) string {
	return ref.Target + ">" + strings.Join(ref.Fields, "+")
}

func fieldsNullable(byName map[string]*schema.Field, fieldNames []string) bool {
	for _, name := range fieldNames {
		f, ok := byName[name]
		if !ok || f.PrimaryKey || f.NotNull {
			return false
		}
	}
	return true
}

func manyToManyJoinEntity(r *schema.Relationship) (autoseed.Entity, error) {
	var fields []autoseed.Field
	for _, f := range r.JoinTable.Fields {
		if f.DBName == "" {
			continue
		}
		fields = append(fields, fieldFrom(f))
	}

	var refs []autoseed.Reference
	for _, ref := range r.References {
		if ref.ForeignKey == nil || ref.PrimaryKey == nil {
			continue
		}
		target := r.FieldSchema.Name
		if ref.OwnPrimaryKey {
			target = r.Schema.Name
		}
		refs = append(refs, autoseed.Reference{
			Fields:   []string{ref.ForeignKey.Name},
			Target:   target,
			Nullable: false,
		})
	}

	if len(refs) != 2 {
		return autoseed.Entity{}, fmt.Errorf("gormseed: many2many join table %q: expected 2 foreign keys, found %d", r.JoinTable.Name, len(refs))
	}

	sort.Slice(refs, func(i, j int) bool { return refs[i].Target < refs[j].Target })
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })

	return autoseed.Entity{Name: r.JoinTable.Name, Fields: fields, References: refs}, nil
}

func fieldFrom(f *schema.Field) autoseed.Field {
	return autoseed.Field{
		Name:          f.Name,
		Type:          f.IndirectFieldType,
		Size:          f.Size,
		Nullable:      !f.PrimaryKey && !f.NotNull && canRepresentNull(f.FieldType),
		Unique:        f.Unique,
		PrimaryKey:    f.PrimaryKey,
		AutoIncrement: f.AutoIncrement,
		SoftDelete:    isSoftDelete(f),
	}
}

func isSoftDelete(f *schema.Field) bool {
	value := reflect.New(f.FieldType).Interface()
	_, ok := value.(schema.QueryClausesInterface)
	return ok
}

// canRepresentNull reports whether t, the field's own (non-indirected) Go
// type, can actually hold a nil value: a pointer, or one of database/sql's
// nullable wrapper types. A plain, non-pointer field GORM's own Field.Set
// silently turns a nil write into that type's zero value, not a real SQL
// NULL, so it must not be marked Nullable regardless of whether the
// column's schema allows NULL.
func canRepresentNull(t reflect.Type) bool {
	return t.Kind() == reflect.Pointer || inference.IsNullableWrapper(t)
}
