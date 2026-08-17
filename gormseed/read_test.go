package gormseed

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
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

// TargetBA's primary key is declared B-then-A, the reverse of the order
// ChildAB's foreignKey/references tag lists them in (A,B). Fields must
// still come out ordered to match TargetBA's own declaration (B,A), not
// the tag's.
type TargetBA struct {
	B int `gorm:"primaryKey"`
	A int `gorm:"primaryKey"`
}

type ChildAB struct {
	ChildID int `gorm:"primaryKey"`
	FA      int
	FB      int
	Target  TargetBA `gorm:"foreignKey:FA,FB;references:A,B"`
}

type Profile struct {
	ID     int `gorm:"primaryKey"`
	UserID int
	Bio    string
}

type User struct {
	ID      int `gorm:"primaryKey"`
	Profile Profile
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
	entities, _, _, err := read(nil, []any{&Post{}, &Author{}})
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
	entities, _, _, err := read(nil, []any{&Cart{}, &CartItem{}})
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
	entities, _, _, err := read(nil, []any{&Post{}, &Author{}})
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
	entities, _, _, err := read(nil, []any{&Post{}, &Author{}, &Tag{}})
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

	postID := fieldNamed(t, join, "PostID")
	tagID := fieldNamed(t, join, "TagID")
	if !postID.PrimaryKey || !tagID.PrimaryKey {
		t.Fatalf("join table key columns not marked PrimaryKey: PostID=%+v TagID=%+v", postID, tagID)
	}
	if postID.Nullable || tagID.Nullable {
		t.Fatalf("join table key columns must not be Nullable: PostID=%+v TagID=%+v", postID, tagID)
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
	entities, _, skipped, err := read(nil, []any{&Video{}, &Comment{}})
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
	entities, _, _, err := read(nil, []any{&Post{}, &Author{}})
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
	entities, _, _, err := read(nil, []any{&Post{}, &Author{}})
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
	entities, _, _, err := read(nil, []any{&Post{}, &Author{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	id := fieldNamed(t, entityNamed(t, entities, "Post"), "ID")
	if !id.PrimaryKey || !id.AutoIncrement || id.Nullable {
		t.Fatalf("Post.ID = %+v, want PrimaryKey=true AutoIncrement=true Nullable=false", id)
	}
}

func TestRead_CompositePrimaryAndForeignKey(t *testing.T) {
	entities, _, _, err := read(nil, []any{&OrderLine{}, &Order{}, &Shipment{}})
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

// TestRead_CompositeReferenceOrderMatchesTargetDeclaration guards a real
// bug: foreignKeyFields used to return fields in the references: tag's own
// order, not TargetBA's declared primary key order. ChildAB's tag lists
// A before B, but TargetBA declares B before A, so the FK fields (FA, FB)
// must come out as [FB, FA] to line up positionally with TargetBA's own
// Fields order.
func TestRead_CompositeReferenceOrderMatchesTargetDeclaration(t *testing.T) {
	entities, _, _, err := read(nil, []any{&ChildAB{}, &TargetBA{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	target := entityNamed(t, entities, "TargetBA")
	if target.Fields[0].Name != "B" || target.Fields[1].Name != "A" {
		t.Fatalf("TargetBA.Fields = %v, want [B A] (declaration order)", fieldNames(target.Fields))
	}

	child := entityNamed(t, entities, "ChildAB")
	ref := referenceTo(t, child, "TargetBA")
	if !equalStrings(ref.Fields, []string{"FB", "FA"}) {
		t.Fatalf("ChildAB -> TargetBA Fields = %v, want [FB FA] (matching TargetBA's own B,A order)", ref.Fields)
	}
}

func TestRead_OneDirectionalHasOne(t *testing.T) {
	entities, _, _, err := read(nil, []any{&User{}, &Profile{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	profile := entityNamed(t, entities, "Profile")
	ref := referenceTo(t, profile, "User")
	if !equalStrings(ref.Fields, []string{"UserID"}) {
		t.Fatalf("Profile -> User Fields = %v, want [UserID]", ref.Fields)
	}
}

func TestRead_SoftDeleteNotGuessedFromFieldName(t *testing.T) {
	type PlainTimestamp struct {
		ID        int `gorm:"primaryKey"`
		DeletedAt time.Time
	}

	entities, _, _, err := read(nil, []any{&PlainTimestamp{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	deletedAt := fieldNamed(t, entityNamed(t, entities, "PlainTimestamp"), "DeletedAt")
	if deletedAt.SoftDelete {
		t.Fatal("a plain time.Time field named DeletedAt must not be marked SoftDelete: detection must go through schema.QueryClausesInterface, never the field name")
	}
}

func TestRead_FieldUnique(t *testing.T) {
	type UniqueEmail struct {
		ID    int    `gorm:"primaryKey"`
		Email string `gorm:"unique"`
	}

	entities, _, _, err := read(nil, []any{&UniqueEmail{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	email := fieldNamed(t, entityNamed(t, entities, "UniqueEmail"), "Email")
	if !email.Unique {
		t.Fatal("Email has a unique tag, want Field.Unique=true")
	}
	id := fieldNamed(t, entityNamed(t, entities, "UniqueEmail"), "ID")
	if id.Unique {
		t.Fatal("ID has no unique tag, want Field.Unique=false")
	}
}

func TestExplain_OmittedModelReturnsUnknownReference(t *testing.T) {
	_, err := Explain(nil, []any{&Post{}})
	if !errors.Is(err, autoseed.ErrUnknownReference) {
		t.Fatalf("got %v, want ErrUnknownReference for a Post whose Author was not included", err)
	}
}

// TestRead_ManyToManyWithExplicitJoinStruct guards a real bug: read() used
// to parse with a fresh cache on every call, so a join-table struct
// registered on db via SetupJoinTable — and any extra column it declares
// beyond the two foreign keys — was invisible, silently replaced by the
// bare two-column table GORM synthesizes by default.
func TestRead_ManyToManyWithExplicitJoinStruct(t *testing.T) {
	type Skill struct {
		ID uint `gorm:"primaryKey"`
	}
	type Person struct {
		ID     uint    `gorm:"primaryKey"`
		Skills []Skill `gorm:"many2many:person_skill_links;"`
	}
	type PersonSkillLink struct {
		PersonID uint `gorm:"primaryKey"`
		SkillID  uint `gorm:"primaryKey"`
		Level    int  `gorm:"not null"`
	}

	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if err := db.SetupJoinTable(&Person{}, "Skills", &PersonSkillLink{}); err != nil {
		t.Fatalf("SetupJoinTable: %v", err)
	}

	entities, _, _, err := read(db, []any{&Person{}, &Skill{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	join := entityNamed(t, entities, "PersonSkillLink")
	level := fieldNamed(t, join, "Level")
	if level.Nullable {
		t.Fatal("Level has a not-null tag, want Nullable=false")
	}
}

func fieldNames(fields []autoseed.Field) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
	}
	return names
}

func TestRead_DuplicateModelInListIsIdempotent(t *testing.T) {
	entities, _, _, err := read(nil, []any{&Order{}, &Order{}})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("entities = %v, want exactly one Order", entityNames(entities))
	}
}

func TestRead_PropagatesParseError(t *testing.T) {
	_, _, _, err := read(nil, []any{42})
	if err == nil {
		t.Fatal("read(42) succeeded, want an error for a non-struct model")
	}
}

func TestRead_Deterministic(t *testing.T) {
	models := []any{&Post{}, &Author{}, &Tag{}, &Video{}, &Comment{}, &Cart{}, &CartItem{}, &OrderLine{}, &Order{}, &Shipment{}}

	first, _, firstSkipped, err := read(nil, models)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	for i := 0; i < 50; i++ {
		entities, _, skipped, err := read(nil, models)
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
