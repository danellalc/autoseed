package gormseed

import (
	"sort"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
	"gorm.io/gorm"
)

type Author struct {
	ID    uint `gorm:"primaryKey"`
	Name  string
	Posts []Post
}

type Address struct {
	Street string
	City   string
}

type Post struct {
	ID        uint `gorm:"primaryKey"`
	Title     string
	AuthorID  uint
	Author    Author
	Address   Address `gorm:"embedded;embeddedPrefix:addr_"`
	Tags      []*Tag  `gorm:"many2many:post_tags;"`
	DeletedAt gorm.DeletedAt
}

type Tag struct {
	ID    uint `gorm:"primaryKey"`
	Name  string
	Posts []*Post `gorm:"many2many:post_tags;"`
}

type Comment struct {
	ID        uint `gorm:"primaryKey"`
	Body      string
	OwnerID   uint
	OwnerType string
}

type Video struct {
	ID       uint `gorm:"primaryKey"`
	Title    string
	Comments []Comment `gorm:"polymorphic:Owner;"`
}

type Cart struct {
	ID    int `gorm:"primaryKey"`
	Items []CartItem
}

type CartItem struct {
	ID     int `gorm:"primaryKey"`
	CartID int
	SKU    string
}

type Order struct {
	ID int `gorm:"primaryKey"`
}

type OrderLine struct {
	OrderID int   `gorm:"primaryKey"`
	LineNo  int   `gorm:"primaryKey"`
	Order   Order `gorm:"foreignKey:OrderID;references:ID"`
	SKU     string
}

type Shipment struct {
	ID          int `gorm:"primaryKey"`
	LineOrderID int
	LineLineNo  int
	OrderLine   OrderLine `gorm:"foreignKey:LineOrderID,LineLineNo;references:OrderID,LineNo"`
}

func entityNamed(t *testing.T, entities []autoseed.Entity, name string) autoseed.Entity {
	t.Helper()
	for _, e := range entities {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no entity named %q in %v", name, entityNames(entities))
	return autoseed.Entity{}
}

func entityNames(entities []autoseed.Entity) []string {
	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = e.Name
	}
	return names
}

func fieldNamed(t *testing.T, entity autoseed.Entity, name string) autoseed.Field {
	t.Helper()
	for _, f := range entity.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("entity %q has no field %q, fields: %v", entity.Name, name, entity.Fields)
	return autoseed.Field{}
}

func referenceTo(t *testing.T, entity autoseed.Entity, target string) autoseed.Reference {
	t.Helper()
	for _, r := range entity.References {
		if r.Target == target {
			return r
		}
	}
	t.Fatalf("entity %q has no reference to %q, references: %v", entity.Name, target, entity.References)
	return autoseed.Reference{}
}

func TestRead_BelongsTo(t *testing.T) {
	entities, _, err := read(nil, []any{&Post{}, &Author{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	post := entityNamed(t, entities, "Post")
	ref := referenceTo(t, post, "Author")
	if !equalStrings(ref.Fields, []string{"AuthorID"}) {
		t.Fatalf("Post -> Author Fields = %v, want [AuthorID]", ref.Fields)
	}
	if !ref.Nullable {
		t.Fatal("Post.AuthorID has no not-null tag, want Nullable=true")
	}
}

// TestRead_OneDirectionalHasMany is the correctness case this adapter
// exists to get right: Cart declares Items []CartItem, but CartItem has no
// field pointing back to Cart. CartItem's own schema has no BelongsTo
// entry for it at all — the reference has to be inferred from Cart's side.
func TestRead_OneDirectionalHasMany(t *testing.T) {
	entities, _, err := read(nil, []any{&Cart{}, &CartItem{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	item := entityNamed(t, entities, "CartItem")
	ref := referenceTo(t, item, "Cart")
	if !equalStrings(ref.Fields, []string{"CartID"}) {
		t.Fatalf("CartItem -> Cart Fields = %v, want [CartID]", ref.Fields)
	}
	if !ref.Nullable {
		t.Fatal("CartItem.CartID has no not-null tag, want Nullable=true")
	}
}

// TestRead_BidirectionalDoesNotDuplicate covers Author.Posts (HasMany) and
// Post.Author (BelongsTo) both describing the same AuthorID -> Author
// relationship: it must appear exactly once on Post.
func TestRead_BidirectionalDoesNotDuplicate(t *testing.T) {
	entities, _, err := read(nil, []any{&Post{}, &Author{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	post := entityNamed(t, entities, "Post")
	count := 0
	for _, r := range post.References {
		if r.Target == "Author" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Post has %d references to Author, want 1: %v", count, post.References)
	}
}

func TestRead_ManyToMany(t *testing.T) {
	entities, _, err := read(nil, []any{&Post{}, &Author{}, &Tag{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	join := entityNamed(t, entities, "post_tags")
	if len(join.References) != 2 {
		t.Fatalf("join table has %d references, want 2: %v", len(join.References), join.References)
	}
	toPost := referenceTo(t, join, "Post")
	toTag := referenceTo(t, join, "Tag")
	if toPost.Nullable || toTag.Nullable {
		t.Fatalf("join table references must be required, got Post.Nullable=%v Tag.Nullable=%v", toPost.Nullable, toTag.Nullable)
	}

	// The join table is discovered from both Post.Tags and Tag.Posts; it
	// must be synthesized exactly once, not twice.
	count := 0
	for _, e := range entities {
		if e.Name == "post_tags" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("post_tags appears %d times, want 1", count)
	}
}

func TestRead_PolymorphicSkippedNamed(t *testing.T) {
	entities, skipped, err := read(nil, []any{&Video{}, &Comment{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	comment := entityNamed(t, entities, "Comment")
	for _, r := range comment.References {
		if r.Target == "Video" {
			t.Fatalf("Comment must not carry a reference for the unresolved polymorphic association, got %v", r)
		}
	}

	if len(skipped) != 1 {
		t.Fatalf("Skipped = %v, want exactly one entry", skipped)
	}
	if skipped[0].Entity != "Comment" || skipped[0].Field != "OwnerID" {
		t.Fatalf("Skipped[0] = %+v, want Entity=Comment Field=OwnerID", skipped[0])
	}
	if !strings.Contains(skipped[0].Reason, "polymorphic") {
		t.Fatalf("Skipped[0].Reason = %q, want it to mention polymorphic", skipped[0].Reason)
	}
}

func TestRead_EmbeddedStructIsColumnsNotEntity(t *testing.T) {
	entities, _, err := read(nil, []any{&Post{}, &Author{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	for _, e := range entities {
		if e.Name == "Address" {
			t.Fatal("embedded struct Address must not become its own entity")
		}
	}

	post := entityNamed(t, entities, "Post")
	fieldNamed(t, post, "Street")
	fieldNamed(t, post, "City")
}

func TestRead_SoftDelete(t *testing.T) {
	entities, _, err := read(nil, []any{&Post{}, &Author{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	post := entityNamed(t, entities, "Post")
	deletedAt := fieldNamed(t, post, "DeletedAt")
	if !deletedAt.SoftDelete {
		t.Fatal("Post.DeletedAt must be marked SoftDelete")
	}

	for _, f := range post.Fields {
		if f.Name != "DeletedAt" && f.SoftDelete {
			t.Fatalf("field %q must not be marked SoftDelete", f.Name)
		}
	}
}

func TestRead_AutoIncrementPrimaryKey(t *testing.T) {
	entities, _, err := read(nil, []any{&Post{}, &Author{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	id := fieldNamed(t, entityNamed(t, entities, "Post"), "ID")
	if !id.PrimaryKey || !id.AutoIncrement || id.Nullable {
		t.Fatalf("Post.ID = %+v, want PrimaryKey=true AutoIncrement=true Nullable=false", id)
	}
}

func TestRead_CompositePrimaryAndForeignKey(t *testing.T) {
	entities, _, err := read(nil, []any{&OrderLine{}, &Order{}, &Shipment{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	orderLine := entityNamed(t, entities, "OrderLine")
	orderID := fieldNamed(t, orderLine, "OrderID")
	lineNo := fieldNamed(t, orderLine, "LineNo")
	if !orderID.PrimaryKey || !lineNo.PrimaryKey {
		t.Fatalf("OrderLine composite key not marked: OrderID=%+v LineNo=%+v", orderID, lineNo)
	}

	shipment := entityNamed(t, entities, "Shipment")
	ref := referenceTo(t, shipment, "OrderLine")
	if !equalStrings(ref.Fields, []string{"LineOrderID", "LineLineNo"}) {
		t.Fatalf("Shipment -> OrderLine Fields = %v, want [LineOrderID LineLineNo]", ref.Fields)
	}
}

func TestRead_DuplicateModelInListIsIdempotent(t *testing.T) {
	entities, _, err := read(nil, []any{&Order{}, &Order{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("entities = %v, want exactly one Order", entityNames(entities))
	}
}

func TestRead_PropagatesParseError(t *testing.T) {
	_, _, err := read(nil, []any{42})
	if err == nil {
		t.Fatal("read(42) succeeded, want an error for a non-struct model")
	}
}

func TestRead_Deterministic(t *testing.T) {
	models := []any{&Post{}, &Author{}, &Tag{}, &Video{}, &Comment{}, &Cart{}, &CartItem{}, &OrderLine{}, &Order{}, &Shipment{}}

	first, firstSkipped, err := read(nil, models)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	for i := 0; i < 50; i++ {
		entities, skipped, err := read(nil, models)
		if err != nil {
			t.Fatalf("read (run %d): %v", i, err)
		}
		if !equalEntities(entities, first) {
			t.Fatalf("run %d: entities differ from the first run", i)
		}
		if !equalStrings(entityNames(skippedEntities(skipped)), entityNames(skippedEntities(firstSkipped))) {
			t.Fatalf("run %d: skipped differs from the first run", i)
		}
	}
}

func skippedEntities(skipped []autoseed.SkipReason) []autoseed.Entity {
	entities := make([]autoseed.Entity, len(skipped))
	for i, s := range skipped {
		entities[i] = autoseed.Entity{Name: s.Entity + "." + s.Field}
	}
	return entities
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalEntities(a, b []autoseed.Entity) bool {
	if len(a) != len(b) {
		return false
	}
	sortEntities(a)
	sortEntities(b)
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		if len(a[i].Fields) != len(b[i].Fields) || len(a[i].References) != len(b[i].References) {
			return false
		}
		for j := range a[i].Fields {
			if a[i].Fields[j] != b[i].Fields[j] {
				return false
			}
		}
		for j := range a[i].References {
			if a[i].References[j].Target != b[i].References[j].Target ||
				a[i].References[j].Nullable != b[i].References[j].Nullable ||
				!equalStrings(a[i].References[j].Fields, b[i].References[j].Fields) {
				return false
			}
		}
	}
	return true
}

func sortEntities(entities []autoseed.Entity) {
	sort.Slice(entities, func(i, j int) bool { return entities[i].Name < entities[j].Name })
}
