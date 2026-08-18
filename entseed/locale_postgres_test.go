package entseed_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/entseed"
)

var ptBRPhonePattern = regexp.MustCompile(`^\+55 \(\d{2}\) 9\d{4}-\d{4}$`)

var ptBRStreetTypePrefixes = []string{"Rua ", "Avenida ", "Travessa ", "Alameda "}

var ptBRCitiesForTest = []string{
	"Sao Paulo", "Rio de Janeiro", "Belo Horizonte", "Brasilia", "Salvador",
	"Fortaleza", "Curitiba", "Recife", "Porto Alegre", "Manaus",
	"Belem", "Goiania", "Guarulhos", "Campinas", "Sao Luis",
	"Maceio", "Natal", "Teresina", "Florianopolis", "Joao Pessoa",
}

// TestSeed_Postgres_LocalePtBR guards the end-to-end path for
// WithLocale("pt_BR") on entseed: Customer's snake_case first_name,
// last_name, city, street and phone all get pt_BR treatment, and
// EmailRule's existing sibling correlation (unmodified by locale
// support) still agrees with whatever Brazilian FirstName/LastName
// landed on the same row.
func TestSeed_Postgres_LocalePtBR(t *testing.T) {
	client := entClient(t)
	ctx := context.Background()

	if err := entseed.Seed(ctx, client, schemaPath, autoseed.WithSeed(6), autoseed.WithScale(30), autoseed.WithLocale("pt_BR")); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	customers, err := client.Customer.Query().All(ctx)
	if err != nil {
		t.Fatalf("querying customers: %v", err)
	}
	if len(customers) != 30 {
		t.Fatalf("Customer count = %d, want 30", len(customers))
	}
	for _, c := range customers {
		if !ptBRPhonePattern.MatchString(c.Phone) {
			t.Fatalf("customer %d: Phone = %q, want the +55 (DDD) 9XXXX-XXXX pt_BR format", c.ID, c.Phone)
		}
		if !hasAnyStreetPrefix(c.Street) {
			t.Fatalf("customer %d: Street = %q, want a pt_BR street type prefix", c.ID, c.Street)
		}
		if !inCitySlice(c.City) {
			t.Fatalf("customer %d: City = %q, not in the pt_BR city pool", c.ID, c.City)
		}
		wantEmailPrefix := strings.ToLower(c.FirstName) + "." + strings.ToLower(c.LastName) + "@"
		if !strings.HasPrefix(c.Email, wantEmailPrefix) {
			t.Fatalf("customer %d: Email = %q, want it to start with %q (agreeing with FirstName/LastName)", c.ID, c.Email, wantEmailPrefix)
		}
	}
}

func hasAnyStreetPrefix(street string) bool {
	for _, prefix := range ptBRStreetTypePrefixes {
		if strings.HasPrefix(street, prefix) {
			return true
		}
	}
	return false
}

func inCitySlice(city string) bool {
	for _, candidate := range ptBRCitiesForTest {
		if city == candidate {
			return true
		}
	}
	return false
}
