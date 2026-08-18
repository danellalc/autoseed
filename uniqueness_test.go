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
