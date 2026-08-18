package inference_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

var ptBRFirstNamesForTest = []string{
	"Joao", "Pedro", "Lucas", "Gabriel", "Matheus", "Rafael", "Gustavo", "Felipe",
	"Bruno", "Rodrigo", "Carlos", "Marcos", "Paulo", "Ricardo", "Eduardo", "Thiago",
	"Diego", "Fernando", "Leonardo", "Andre",
	"Maria", "Ana", "Juliana", "Fernanda", "Camila", "Amanda", "Beatriz", "Larissa",
	"Patricia", "Aline", "Bruna", "Carla", "Daniela", "Renata", "Vanessa", "Priscila",
	"Tatiane", "Cristina", "Adriana", "Luciana",
}

var ptBRLastNamesForTest = []string{
	"Silva", "Santos", "Oliveira", "Souza", "Rodrigues", "Ferreira", "Alves", "Pereira",
	"Lima", "Gomes", "Costa", "Ribeiro", "Martins", "Carvalho", "Almeida", "Lopes",
	"Soares", "Fernandes", "Vieira", "Barbosa", "Rocha", "Dias", "Nunes", "Moreira",
	"Nascimento", "Cardoso", "Correia", "Teixeira", "Araujo", "Melo",
}

var ptBRCitiesForTest = []string{
	"Sao Paulo", "Rio de Janeiro", "Belo Horizonte", "Brasilia", "Salvador",
	"Fortaleza", "Curitiba", "Recife", "Porto Alegre", "Manaus",
	"Belem", "Goiania", "Guarulhos", "Campinas", "Sao Luis",
	"Maceio", "Natal", "Teresina", "Florianopolis", "Joao Pessoa",
}

var ptBRStreetTypesForTest = []string{"Rua", "Avenida", "Travessa", "Alameda"}

func TestPtBRNameRule_CanInfer(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want bool
	}{
		{"FirstName", reflect.TypeOf(""), true},
		{"LastName", reflect.TypeOf(""), true},
		{"OwnerFirstName", reflect.TypeOf(""), true},
		{"FirstName", reflect.TypeOf(0), false},
		{"Email", reflect.TypeOf(""), false},
	}
	for _, tt := range tests {
		field := autoseed.Field{Name: tt.name, Type: tt.typ}
		if got := (inference.PtBRNameRule{}).CanInfer(field); got != tt.want {
			t.Errorf("CanInfer(%s %s) = %v, want %v", tt.name, tt.typ, got, tt.want)
		}
	}
}

// TestPtBRNameRule_ProducesNamesFromPool guards both that every value
// comes from the pool AND that the seed actually drives which one — a
// rule that ignored its seed and always returned the same pool entry
// would still pass a pool-membership-only check.
func TestPtBRNameRule_ProducesNamesFromPool(t *testing.T) {
	firstName := autoseed.Field{Name: "FirstName", Type: reflect.TypeOf("")}
	lastName := autoseed.Field{Name: "LastName", Type: reflect.TypeOf("")}
	seenFirst := make(map[string]bool)
	seenLast := make(map[string]bool)

	for seed := uint64(0); seed < 200; seed++ {
		got := inferField(t, inference.PtBRNameRule{}, firstName, seed).(string)
		if !inSlice(got, ptBRFirstNamesForTest) {
			t.Fatalf("seed %d: FirstName = %q, not in the pt_BR first-name pool", seed, got)
		}
		seenFirst[got] = true

		got = inferField(t, inference.PtBRNameRule{}, lastName, seed).(string)
		if !inSlice(got, ptBRLastNamesForTest) {
			t.Fatalf("seed %d: LastName = %q, not in the pt_BR last-name pool", seed, got)
		}
		seenLast[got] = true
	}

	if len(seenFirst) < 5 {
		t.Fatalf("only %d distinct FirstName values over 200 seeds, want the seed to actually vary the pick: %v", len(seenFirst), seenFirst)
	}
	if len(seenLast) < 5 {
		t.Fatalf("only %d distinct LastName values over 200 seeds, want the seed to actually vary the pick: %v", len(seenLast), seenLast)
	}
}

func TestPtBRAddressRule_CanInfer(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want bool
	}{
		{"City", reflect.TypeOf(""), true},
		{"Street", reflect.TypeOf(""), true},
		{"StreetName", reflect.TypeOf(""), true},
		{"Address", reflect.TypeOf(""), true},
		{"EmailAddress", reflect.TypeOf(""), false},
		{"City", reflect.TypeOf(0), false},
	}
	for _, tt := range tests {
		field := autoseed.Field{Name: tt.name, Type: tt.typ}
		if got := (inference.PtBRAddressRule{}).CanInfer(field); got != tt.want {
			t.Errorf("CanInfer(%s %s) = %v, want %v", tt.name, tt.typ, got, tt.want)
		}
	}
}

// TestPtBRAddressRule_ProducesValuesFromPool guards both pool/format
// membership AND that the seed actually drives the pick, the same
// diversity guard TestPtBRNameRule_ProducesNamesFromPool applies.
func TestPtBRAddressRule_ProducesValuesFromPool(t *testing.T) {
	city := autoseed.Field{Name: "City", Type: reflect.TypeOf("")}
	street := autoseed.Field{Name: "Street", Type: reflect.TypeOf("")}
	seenCity := make(map[string]bool)
	seenStreet := make(map[string]bool)

	for seed := uint64(0); seed < 200; seed++ {
		got := inferField(t, inference.PtBRAddressRule{}, city, seed).(string)
		if !inSlice(got, ptBRCitiesForTest) {
			t.Fatalf("seed %d: City = %q, not in the pt_BR city pool", seed, got)
		}
		seenCity[got] = true

		got = inferField(t, inference.PtBRAddressRule{}, street, seed).(string)
		if !hasAnyPrefix(got, ptBRStreetTypesForTest) {
			t.Fatalf("seed %d: Street = %q, want a pt_BR street type prefix", seed, got)
		}
		seenStreet[got] = true
	}

	if len(seenCity) < 5 {
		t.Fatalf("only %d distinct City values over 200 seeds, want the seed to actually vary the pick: %v", len(seenCity), seenCity)
	}
	if len(seenStreet) < 5 {
		t.Fatalf("only %d distinct Street values over 200 seeds, want the seed to actually vary the pick: %v", len(seenStreet), seenStreet)
	}
}

func TestPtBRPhoneRule_CanInfer(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want bool
	}{
		{"Phone", reflect.TypeOf(""), true},
		{"PhoneNumber", reflect.TypeOf(""), true},
		{"Phone", reflect.TypeOf(0), false},
		{"City", reflect.TypeOf(""), false},
	}
	for _, tt := range tests {
		field := autoseed.Field{Name: tt.name, Type: tt.typ}
		if got := (inference.PtBRPhoneRule{}).CanInfer(field); got != tt.want {
			t.Errorf("CanInfer(%s %s) = %v, want %v", tt.name, tt.typ, got, tt.want)
		}
	}
}

// TestPtBRPhoneRule_MatchesBrazilianMobileFormat guards both the format
// AND that the seed actually drives the number, the same diversity guard
// TestPtBRNameRule_ProducesNamesFromPool applies.
func TestPtBRPhoneRule_MatchesBrazilianMobileFormat(t *testing.T) {
	field := autoseed.Field{Name: "Phone", Type: reflect.TypeOf("")}
	pattern := regexp.MustCompile(`^\+55 \(\d{2}\) 9\d{4}-\d{4}$`)
	seen := make(map[string]bool)

	for seed := uint64(0); seed < 200; seed++ {
		got := inferField(t, inference.PtBRPhoneRule{}, field, seed).(string)
		if !pattern.MatchString(got) {
			t.Fatalf("seed %d: Phone = %q, want the +55 (DDD) 9XXXX-XXXX pt_BR format", seed, got)
		}
		seen[got] = true
	}

	if len(seen) < 5 {
		t.Fatalf("only %d distinct Phone values over 200 seeds, want the seed to actually vary the number: %v", len(seen), seen)
	}
}

// TestGenerator_WithLocale_PtBRClaimsBeforeGenericRules guards that
// every one of PtBRNameRule/PtBRAddressRule/PtBRPhoneRule's Priority -1
// actually wins the claim over its generic, Priority 0+ counterpart for
// every field it covers — including Phone, easy to omit since it's the
// one field this test only needed to include so GenerateRow wouldn't
// error on an unclaimed field, not for its own sake — and that the seed
// actually drives each pick, not just that some value from the pool
// eventually shows up.
func TestGenerator_WithLocale_PtBRClaimsBeforeGenericRules(t *testing.T) {
	gen := inference.NewDefaultGenerator().WithLocale("pt_BR")
	entity := autoseed.Entity{
		Name: "Customer",
		Fields: []autoseed.Field{
			{Name: "FirstName", Type: reflect.TypeOf("")},
			{Name: "LastName", Type: reflect.TypeOf("")},
			{Name: "City", Type: reflect.TypeOf("")},
			{Name: "Street", Type: reflect.TypeOf("")},
			{Name: "Phone", Type: reflect.TypeOf("")},
		},
	}
	phonePattern := regexp.MustCompile(`^\+55 \(\d{2}\) 9\d{4}-\d{4}$`)
	seenFirst := make(map[string]bool)
	seenPhone := make(map[string]bool)

	for seed := uint64(0); seed < 100; seed++ {
		values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(seed).Entity("Customer").Row(0))
		if err != nil {
			t.Fatalf("seed %d: GenerateRow: %v", seed, err)
		}
		if !inSlice(values["FirstName"].(string), ptBRFirstNamesForTest) {
			t.Fatalf("seed %d: FirstName = %v, not from the pt_BR pool", seed, values["FirstName"])
		}
		seenFirst[values["FirstName"].(string)] = true
		if !inSlice(values["LastName"].(string), ptBRLastNamesForTest) {
			t.Fatalf("seed %d: LastName = %v, not from the pt_BR pool", seed, values["LastName"])
		}
		if !inSlice(values["City"].(string), ptBRCitiesForTest) {
			t.Fatalf("seed %d: City = %v, not from the pt_BR pool", seed, values["City"])
		}
		if !hasAnyPrefix(values["Street"].(string), ptBRStreetTypesForTest) {
			t.Fatalf("seed %d: Street = %v, want a pt_BR street type prefix", seed, values["Street"])
		}
		phone := values["Phone"].(string)
		if !phonePattern.MatchString(phone) {
			t.Fatalf("seed %d: Phone = %q, want the +55 (DDD) 9XXXX-XXXX pt_BR format -- PtBRPhoneRule must claim it, not the generic PhoneRule", seed, phone)
		}
		seenPhone[phone] = true
	}

	if len(seenFirst) < 5 {
		t.Fatalf("only %d distinct FirstName values over 100 seeds, want the seed to actually vary the pick: %v", len(seenFirst), seenFirst)
	}
	if len(seenPhone) < 5 {
		t.Fatalf("only %d distinct Phone values over 100 seeds, want the seed to actually vary the number: %v", len(seenPhone), seenPhone)
	}
}

// TestGenerator_WithLocale_EmailAgreesWithPtBRName guards that EmailRule's
// existing sibling correlation carries through transparently: it reads
// whatever FirstName/LastName ended up in the row, with no locale
// awareness of its own, so a pt_BR name should already produce a
// matching "firstname.lastname@domain" email with zero changes to
// EmailRule itself.
func TestGenerator_WithLocale_EmailAgreesWithPtBRName(t *testing.T) {
	gen := inference.NewDefaultGenerator().WithLocale("pt_BR")
	entity := autoseed.Entity{
		Name: "Customer",
		Fields: []autoseed.Field{
			{Name: "FirstName", Type: reflect.TypeOf("")},
			{Name: "LastName", Type: reflect.TypeOf("")},
			{Name: "Email", Type: reflect.TypeOf("")},
		},
	}

	for seed := uint64(0); seed < 50; seed++ {
		values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(seed).Entity("Customer").Row(0))
		if err != nil {
			t.Fatalf("seed %d: GenerateRow: %v", seed, err)
		}
		first := strings.ToLower(values["FirstName"].(string))
		last := strings.ToLower(values["LastName"].(string))
		email := values["Email"].(string)
		wantPrefix := first + "." + last + "@"
		if !strings.HasPrefix(email, wantPrefix) {
			t.Fatalf("seed %d: Email = %q, want it to start with %q", seed, email, wantPrefix)
		}
	}
}

// TestGenerator_WithLocale_UnrecognizedFallsBackToGeneric guards that an
// unknown locale string changes nothing: the generic rules still claim
// every field, the same as if WithLocale were never called.
func TestGenerator_WithLocale_UnrecognizedFallsBackToGeneric(t *testing.T) {
	entity := autoseed.Entity{
		Name:   "Customer",
		Fields: []autoseed.Field{{Name: "FirstName", Type: reflect.TypeOf("")}},
	}

	withUnknown, err := inference.NewDefaultGenerator().WithLocale("xx_XX").GenerateRow(entity, autoseed.NewSeededSource(1).Entity("Customer").Row(0))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	withNone, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(1).Entity("Customer").Row(0))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	if withUnknown["FirstName"] != withNone["FirstName"] {
		t.Fatalf("an unrecognized locale changed the generated value: %v vs %v", withUnknown["FirstName"], withNone["FirstName"])
	}
}

func TestGenerator_WithLocale_PtBRDeterministic(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Customer",
		Fields: []autoseed.Field{
			{Name: "FirstName", Type: reflect.TypeOf("")},
			{Name: "Phone", Type: reflect.TypeOf("")},
		},
	}
	root := autoseed.NewSeededSource(42)

	build := func() map[string]any {
		gen := inference.NewDefaultGenerator().WithLocale("pt_BR")
		values, err := gen.GenerateRow(entity, root.Entity("Customer").Row(3))
		if err != nil {
			t.Fatalf("GenerateRow: %v", err)
		}
		return values
	}

	first := build()
	for i := 0; i < 20; i++ {
		got := build()
		if got["FirstName"] != first["FirstName"] || got["Phone"] != first["Phone"] {
			t.Fatalf("run %d: values differ from the first run: %v vs %v", i, got, first)
		}
	}
}

func inSlice(value string, pool []string) bool {
	for _, candidate := range pool {
		if value == candidate {
			return true
		}
	}
	return false
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix+" ") {
			return true
		}
	}
	return false
}
