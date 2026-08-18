package autoseed_test

import (
	"errors"
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/danellalc/autoseed"
)

func stringField(name string, unique bool, size int) autoseed.Field {
	return autoseed.Field{Name: name, Type: reflect.TypeOf(""), Unique: unique, Size: size}
}

func TestEnsureUnique_NoDuplicatesUntouched(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Email", true, 0)}}
	rows := []map[string]any{
		{"Email": "a@example.com"},
		{"Email": "b@example.com"},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[0]["Email"] != "a@example.com" || rows[1]["Email"] != "b@example.com" {
		t.Fatalf("rows changed unexpectedly: %v", rows)
	}
}

func TestEnsureUnique_RewritesDuplicates(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Email", true, 0)}}
	rows := []map[string]any{
		{"Email": "same@example.com"},
		{"Email": "same@example.com"},
		{"Email": "same@example.com"},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}

	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		value := row["Email"].(string)
		if seen[value] {
			t.Fatalf("duplicate value %q survived: %v", value, rows)
		}
		seen[value] = true
	}
	if rows[0]["Email"] != "same@example.com" {
		t.Fatalf("first occurrence should keep its original value, got %v", rows[0]["Email"])
	}
}

func TestEnsureUnique_RespectsSizeLimit(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Code", true, 6)}}
	rows := []map[string]any{
		{"Code": "ABCDEF"},
		{"Code": "ABCDEF"},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if got := rows[1]["Code"].(string); len(got) > 6 {
		t.Fatalf("rewritten value %q exceeds Size=6", got)
	}
}

func TestEnsureUnique_NonUniqueFieldUntouched(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Nickname", false, 0)}}
	rows := []map[string]any{{"Nickname": "same"}, {"Nickname": "same"}}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[0]["Nickname"] != "same" || rows[1]["Nickname"] != "same" {
		t.Fatalf("non-unique field must not be rewritten, got %v", rows)
	}
}

func TestEnsureUnique_NonStringUniqueFieldUntouched(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{
		{Name: "Code", Type: reflect.TypeOf(0), Unique: true},
	}}
	rows := []map[string]any{{"Code": 1}, {"Code": 1}}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[0]["Code"] != 1 || rows[1]["Code"] != 1 {
		t.Fatalf("non-string unique field is out of scope, must not be rewritten, got %v", rows)
	}
}

func TestEnsureUnique_NilSeedReturnsError(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Email", true, 0)}}
	rows := []map[string]any{{"Email": "a@example.com"}, {"Email": "a@example.com"}}

	err := autoseed.EnsureUnique(entity, rows, nil)
	if !errors.Is(err, autoseed.ErrNilSeed) {
		t.Fatalf("got %v, want ErrNilSeed", err)
	}
}

// TestEnsureUnique_RewrittenValueIsValidUTF8WithinRuneSize guards a real
// bug: the rewritten value's prefix used to be cut with a byte index
// (value[:n]), not a rune index. Field.Size models a SQL VARCHAR(n)
// character limit, so a cut landing inside a multi-byte rune produced a
// value that was both over the character limit by the database's own
// counting and not even valid UTF-8. Runs across many seeds so the
// random suffix length varies the exact cut point relative to the
// multi-byte character in "café".
func TestEnsureUnique_RewrittenValueIsValidUTF8WithinRuneSize(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Name", true, 5)}}

	for seed := uint64(0); seed < 200; seed++ {
		rows := []map[string]any{{"Name": "café"}, {"Name": "café"}}
		if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(seed)); err != nil {
			t.Fatalf("seed %d: EnsureUnique: %v", seed, err)
		}
		got := rows[1]["Name"].(string)
		if !utf8.ValidString(got) {
			t.Fatalf("seed %d: rewritten value %q is not valid UTF-8", seed, got)
		}
		if runes := len([]rune(got)); runes > 5 {
			t.Fatalf("seed %d: rewritten value %q has %d runes, want at most 5", seed, got, runes)
		}
	}
}

func TestEnsureUnique_ExhaustsRetriesReturnsNamedError(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Flag", true, 1)}}
	rows := make([]map[string]any, 200)
	for i := range rows {
		rows[i] = map[string]any{"Flag": "X"}
	}

	err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1))
	if !errors.Is(err, autoseed.ErrUnsatisfiableUniqueness) {
		t.Fatalf("got %v, want ErrUnsatisfiableUniqueness", err)
	}
}

func TestEnsureUnique_CompositeConstraintRewritesDuplicateTuple(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Employee",
		Fields: []autoseed.Field{
			stringField("FirstName", false, 0),
			stringField("LastName", false, 0),
		},
		UniqueConstraints: [][]string{{"FirstName", "LastName"}},
	}
	rows := []map[string]any{
		{"FirstName": "Ana", "LastName": "Silva"},
		{"FirstName": "Ana", "LastName": "Silva"},
		{"FirstName": "Bruno", "LastName": "Silva"},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}

	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		key := row["FirstName"].(string) + "|" + row["LastName"].(string)
		if seen[key] {
			t.Fatalf("duplicate tuple %q survived: %v", key, rows)
		}
		seen[key] = true
	}
	if rows[0]["FirstName"] != "Ana" || rows[0]["LastName"] != "Silva" {
		t.Fatalf("first occurrence should keep its original tuple, got %v", rows[0])
	}
	if rows[2]["FirstName"] != "Bruno" || rows[2]["LastName"] != "Silva" {
		t.Fatalf("row with a distinct tuple must not be rewritten, got %v", rows[2])
	}
}

func TestEnsureUnique_CompositeConstraintRewritesLastField(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Employee",
		Fields: []autoseed.Field{
			stringField("FirstName", false, 0),
			stringField("LastName", false, 0),
		},
		UniqueConstraints: [][]string{{"FirstName", "LastName"}},
	}
	rows := []map[string]any{
		{"FirstName": "Ana", "LastName": "Silva"},
		{"FirstName": "Ana", "LastName": "Silva"},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[1]["FirstName"] != "Ana" {
		t.Fatalf("only the constraint's last field should be rewritten, got FirstName=%v", rows[1]["FirstName"])
	}
	if rows[1]["LastName"] == "Silva" {
		t.Fatalf("LastName was not rewritten despite the tuple colliding, got %v", rows[1])
	}
}

// TestEnsureUnique_CompositeConstraintNilFieldNeverCollides guards
// constraintKey's nil handling: a Nullable field null-rate left out of
// two different rows must never make EnsureUnique treat their tuples as
// colliding, the same way two real SQL NULLs are never equal to each
// other under a unique constraint. Before this guard, two rows both
// holding nil for LastName would have produced the identical key
// "Ana\x1f<nil>" and been flagged as a duplicate — then failed outright,
// since a nil LastName also fails the rewrite target's own string type
// assertion.
func TestEnsureUnique_CompositeConstraintNilFieldNeverCollides(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Employee",
		Fields: []autoseed.Field{
			stringField("FirstName", false, 0),
			stringField("LastName", false, 0),
		},
		UniqueConstraints: [][]string{{"FirstName", "LastName"}},
	}
	rows := []map[string]any{
		{"FirstName": "Ana", "LastName": nil},
		{"FirstName": "Ana", "LastName": nil},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[0]["LastName"] != nil || rows[1]["LastName"] != nil {
		t.Fatalf("a row with a nil constraint field must never be rewritten, got %v", rows)
	}
}

// TestEnsureUnique_CompositeConstraintNilRowsDoNotMaskRealDuplicates
// guards that the nil-tuple exemption doesn't disable real-collision
// detection for the OTHER rows sharing the same constraint: a batch
// mixing nil-LastName rows with a genuine (Ana, Silva) duplicate must
// still dedup the real duplicate while leaving the nil rows alone.
func TestEnsureUnique_CompositeConstraintNilRowsDoNotMaskRealDuplicates(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Employee",
		Fields: []autoseed.Field{
			stringField("FirstName", false, 0),
			stringField("LastName", false, 0),
		},
		UniqueConstraints: [][]string{{"FirstName", "LastName"}},
	}
	rows := []map[string]any{
		{"FirstName": "Ana", "LastName": nil},
		{"FirstName": "Ana", "LastName": "Silva"},
		{"FirstName": "Ana", "LastName": nil},
		{"FirstName": "Ana", "LastName": "Silva"},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[0]["LastName"] != nil || rows[2]["LastName"] != nil {
		t.Fatalf("nil rows must never be rewritten, got %v", rows)
	}
	if rows[1]["LastName"] != "Silva" {
		t.Fatalf("the first real occurrence must keep its value, got %v", rows[1])
	}
	if rows[3]["LastName"] == "Silva" {
		t.Fatalf("the real duplicate (Ana, Silva) must still be rewritten, got %v", rows[3])
	}
}

func TestEnsureUnique_CompositeConstraintSpanningReferenceFieldUntouched(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Enrollment",
		Fields: []autoseed.Field{
			{Name: "StudentID", Type: reflect.TypeOf(0)},
			{Name: "CourseID", Type: reflect.TypeOf(0)},
		},
		References: []autoseed.Reference{
			{Fields: []string{"StudentID"}, Target: "Student"},
			{Fields: []string{"CourseID"}, Target: "Course"},
		},
		UniqueConstraints: [][]string{{"StudentID", "CourseID"}},
	}
	rows := []map[string]any{
		{"StudentID": 1, "CourseID": 1},
		{"StudentID": 1, "CourseID": 1},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[0]["StudentID"] != 1 || rows[0]["CourseID"] != 1 || rows[1]["StudentID"] != 1 || rows[1]["CourseID"] != 1 {
		t.Fatalf("a constraint spanning only reference fields is out of scope, must not be rewritten, got %v", rows)
	}
}

func TestEnsureUnique_CompositeConstraintAllNonStringUntouched(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Measurement",
		Fields: []autoseed.Field{
			{Name: "SensorID", Type: reflect.TypeOf(0)},
			{Name: "ReadingIndex", Type: reflect.TypeOf(0)},
		},
		UniqueConstraints: [][]string{{"SensorID", "ReadingIndex"}},
	}
	rows := []map[string]any{
		{"SensorID": 1, "ReadingIndex": 1},
		{"SensorID": 1, "ReadingIndex": 1},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[1]["SensorID"] != 1 || rows[1]["ReadingIndex"] != 1 {
		t.Fatalf("a constraint with no string field to rewrite is out of scope, got %v", rows[1])
	}
}

// TestEnsureUnique_CompositeConstraintExhaustsRetriesReturnsNamedError
// guards genuine retry-loop exhaustion, not just a field with no room for
// any suffix at all: LastName's Size=2 admits exactly 10 distinct
// "-0".."-9" rewrites (11 counting the kept original), so 200 rows all
// starting identical guarantees the candidate pool itself runs out within
// the 20-attempt budget, not the zero-room short-circuit a Size=1 field
// would hit on its very first attempt.
// TestEnsureUnique_CompositeConstraintTrailingNonStringFieldStillRewritesLastString
// guards that rewritableConstraintField picks the last STRING-typed
// field in the constraint, not simply the constraint's last-listed
// field: Priority (an int) trails the tuple, so Category, not Priority,
// must be the one rewritten on a collision.
func TestEnsureUnique_CompositeConstraintTrailingNonStringFieldStillRewritesLastString(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Ticket",
		Fields: []autoseed.Field{
			stringField("Code", false, 0),
			stringField("Category", false, 0),
			{Name: "Priority", Type: reflect.TypeOf(0)},
		},
		UniqueConstraints: [][]string{{"Code", "Category", "Priority"}},
	}
	rows := []map[string]any{
		{"Code": "A", "Category": "bug", "Priority": 1},
		{"Code": "A", "Category": "bug", "Priority": 1},
	}

	if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}
	if rows[1]["Code"] != "A" {
		t.Fatalf("Code must not be rewritten, got %v", rows[1]["Code"])
	}
	if rows[1]["Priority"] != 1 {
		t.Fatalf("Priority is not a string field and must not be rewritten, got %v", rows[1]["Priority"])
	}
	if rows[1]["Category"] == "bug" {
		t.Fatalf("Category (the last string field) was not rewritten despite the tuple colliding, got %v", rows[1])
	}
}

// TestEnsureUnique_TwoConstraintsSharingARewriteTargetFieldStayConsistent
// guards a real bug found in review: two UniqueConstraints entries that
// share the same rewritable field (B, here) used to be resolved by two
// fully independent passes, so a later pass rewriting B to fix its own
// constraint could silently reintroduce a duplicate an earlier pass had
// already fixed under B's OTHER constraint, with EnsureUnique still
// returning nil. Every row starts identical across a small A/C grid with
// a tiny Size on B, forcing heavy rewrite pressure and cross-constraint
// interaction, across many seeds so the interaction is exercised even
// though which rows collide with which is randomized.
func TestEnsureUnique_TwoConstraintsSharingARewriteTargetFieldStayConsistent(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Assignment",
		Fields: []autoseed.Field{
			stringField("A", false, 0),
			stringField("B", false, 2),
			stringField("C", false, 0),
		},
		UniqueConstraints: [][]string{{"A", "B"}, {"C", "B"}},
	}

	for seed := uint64(0); seed < 200; seed++ {
		rows := make([]map[string]any, 6)
		for i := range rows {
			a := "a1"
			if i%2 == 1 {
				a = "a2"
			}
			c := "c1"
			if (i/2)%2 == 1 {
				c = "c2"
			}
			rows[i] = map[string]any{"A": a, "B": "dup", "C": c}
		}

		if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(seed)); err != nil {
			t.Fatalf("seed %d: EnsureUnique: %v", seed, err)
		}

		seenAB := make(map[string]bool, len(rows))
		seenCB := make(map[string]bool, len(rows))
		for _, row := range rows {
			ab := row["A"].(string) + "|" + row["B"].(string)
			cb := row["C"].(string) + "|" + row["B"].(string)
			if seenAB[ab] {
				t.Fatalf("seed %d: duplicate (A,B) = %q survived, rows: %v", seed, ab, rows)
			}
			if seenCB[cb] {
				t.Fatalf("seed %d: duplicate (C,B) = %q survived, rows: %v", seed, cb, rows)
			}
			seenAB[ab] = true
			seenCB[cb] = true
		}
	}
}

func TestEnsureUnique_CompositeConstraintExhaustsRetriesReturnsNamedError(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Employee",
		Fields: []autoseed.Field{
			stringField("FirstName", false, 0),
			stringField("LastName", false, 2),
		},
		UniqueConstraints: [][]string{{"FirstName", "LastName"}},
	}
	rows := make([]map[string]any, 200)
	for i := range rows {
		rows[i] = map[string]any{"FirstName": "Ana", "LastName": "X"}
	}

	err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(1))
	if !errors.Is(err, autoseed.ErrUnsatisfiableUniqueness) {
		t.Fatalf("got %v, want ErrUnsatisfiableUniqueness", err)
	}
}

func TestEnsureUnique_CompositeConstraintDeterministic(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Employee",
		Fields: []autoseed.Field{
			stringField("FirstName", false, 0),
			stringField("LastName", false, 0),
		},
		UniqueConstraints: [][]string{{"FirstName", "LastName"}},
	}

	build := func() []map[string]any {
		return []map[string]any{
			{"FirstName": "Ana", "LastName": "Silva"},
			{"FirstName": "Ana", "LastName": "Silva"},
			{"FirstName": "Ana", "LastName": "Silva"},
		}
	}

	first := build()
	if err := autoseed.EnsureUnique(entity, first, autoseed.NewSeededSource(42)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}

	for i := 0; i < 20; i++ {
		rows := build()
		if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(42)); err != nil {
			t.Fatalf("run %d: EnsureUnique: %v", i, err)
		}
		for j := range rows {
			if rows[j]["LastName"] != first[j]["LastName"] {
				t.Fatalf("run %d: row %d LastName = %v, want %v", i, j, rows[j]["LastName"], first[j]["LastName"])
			}
		}
	}
}

func TestEnsureUnique_Deterministic(t *testing.T) {
	entity := autoseed.Entity{Name: "User", Fields: []autoseed.Field{stringField("Email", true, 0)}}

	build := func() []map[string]any {
		return []map[string]any{{"Email": "dup"}, {"Email": "dup"}, {"Email": "dup"}}
	}

	first := build()
	if err := autoseed.EnsureUnique(entity, first, autoseed.NewSeededSource(42)); err != nil {
		t.Fatalf("EnsureUnique: %v", err)
	}

	for i := 0; i < 20; i++ {
		rows := build()
		if err := autoseed.EnsureUnique(entity, rows, autoseed.NewSeededSource(42)); err != nil {
			t.Fatalf("run %d: EnsureUnique: %v", i, err)
		}
		for j := range rows {
			if rows[j]["Email"] != first[j]["Email"] {
				t.Fatalf("run %d: row %d = %v, want %v", i, j, rows[j]["Email"], first[j]["Email"])
			}
		}
	}
}
