package gormseed

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/danellalc/autoseed"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// inferredReference is a reference discovered from the owning side of a
// HasMany or HasOne relationship, attributed to the child entity that
// actually carries the foreign key column.
type inferredReference struct {
	fields []string
	target string
}

func read(db *gorm.DB, models []any) ([]autoseed.Entity, []autoseed.SkipReason, error) {
	namer := namingStrategy(db)
	cache := &sync.Map{}

	schemas, order, err := parseAll(models, cache, namer)
	if err != nil {
		return nil, nil, err
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

			fields, err := foreignKeyFields(r)
			if err != nil {
				return nil, nil, err
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
				return nil, nil, err
			}
			joinTables[entity.Name] = entity
		}
	}

	entities := make([]autoseed.Entity, 0, len(order)+len(joinTables))
	for _, name := range order {
		entity, err := buildEntity(schemas[name], inferredByChild[name])
		if err != nil {
			return nil, nil, err
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

	return entities, skipped, nil
}

func namingStrategy(db *gorm.DB) schema.Namer {
	if db != nil && db.NamingStrategy != nil {
		return db.NamingStrategy
	}
	return schema.NamingStrategy{}
}

// parseAll parses every model into its schema, in models order, skipping a
// struct type already seen. The returned order is the deduplicated
// discovery order, used later so output does not depend on map iteration.
func parseAll(models []any, cache *sync.Map, namer schema.Namer) (map[string]*schema.Schema, []string, error) {
	schemas := make(map[string]*schema.Schema, len(models))
	order := make([]string, 0, len(models))

	for _, model := range models {
		s, err := schema.Parse(model, cache, namer)
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

// childOwnedRelationships returns the relationships whose foreign key
// column lives on the related entity rather than on s: HasMany and HasOne.
// This is how the common one-directional shape — a parent declaring
// `Items []Item`, with no reciprocal field on Item — surfaces at all: Item
// itself has no BelongsTo entry for it.
func childOwnedRelationships(s *schema.Schema) []*schema.Relationship {
	rels := make([]*schema.Relationship, 0, len(s.Relationships.HasMany)+len(s.Relationships.HasOne))
	rels = append(rels, s.Relationships.HasMany...)
	rels = append(rels, s.Relationships.HasOne...)
	return rels
}

// foreignKeyFields extracts the foreign key column names from a
// relationship's references, in declaration order: one field for a simple
// foreign key, several for a composite one.
func foreignKeyFields(r *schema.Relationship) ([]string, error) {
	fields := make([]string, 0, len(r.References))
	for _, ref := range r.References {
		if ref.ForeignKey == nil {
			continue
		}
		fields = append(fields, ref.ForeignKey.Name)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("gormseed: relationship %q on %q has no foreign key fields", r.Name, r.Schema.Name)
	}
	return fields, nil
}

// buildEntity translates one parsed schema into an autoseed.Entity.
// inferred carries references discovered from the other side of a
// one-directional HasMany or HasOne, merged in only when s's own BelongsTo
// relationships did not already report the same reference.
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
		fkFields, err := foreignKeyFields(r)
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

	return autoseed.Entity{Name: s.Name, Fields: fields, References: refs}, nil
}

func referenceKey(ref autoseed.Reference) string {
	return ref.Target + ">" + strings.Join(ref.Fields, "+")
}

// fieldsNullable reports whether every named field allows null: a
// composite reference is only as nullable as its strictest column.
func fieldsNullable(byName map[string]*schema.Field, fieldNames []string) bool {
	for _, name := range fieldNames {
		f, ok := byName[name]
		if !ok || f.PrimaryKey || f.NotNull {
			return false
		}
	}
	return true
}

// manyToManyJoinEntity synthesizes the join table a many2many relationship
// implies as its own autoseed.Entity, with a required reference to each
// side. GORM reports the same join table symmetrically from both related
// schemas; the caller only needs to call this once, for whichever side it
// reaches first.
func manyToManyJoinEntity(r *schema.Relationship) (autoseed.Entity, error) {
	var fields []autoseed.Field
	var refs []autoseed.Reference

	for _, ref := range r.References {
		if ref.ForeignKey == nil || ref.PrimaryKey == nil {
			continue
		}
		fields = append(fields, fieldFrom(ref.ForeignKey))

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
		Nullable:      !f.PrimaryKey && !f.NotNull,
		Unique:        f.Unique,
		PrimaryKey:    f.PrimaryKey,
		AutoIncrement: f.AutoIncrement,
		SoftDelete:    isSoftDelete(f),
	}
}

// isSoftDelete reports whether f's type is the one GORM itself uses to
// decide a field filters every query — gorm.DeletedAt and anything with
// the same shape, never guessed from the field's name.
func isSoftDelete(f *schema.Field) bool {
	value := reflect.New(f.FieldType).Interface()
	_, ok := value.(schema.QueryClausesInterface)
	return ok
}
