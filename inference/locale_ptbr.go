package inference

import (
	"fmt"
	"reflect"

	"github.com/danellalc/autoseed"
)

var ptBRFirstNames = []string{
	"Joao", "Pedro", "Lucas", "Gabriel", "Matheus", "Rafael", "Gustavo", "Felipe",
	"Bruno", "Rodrigo", "Carlos", "Marcos", "Paulo", "Ricardo", "Eduardo", "Thiago",
	"Diego", "Fernando", "Leonardo", "Andre",
	"Maria", "Ana", "Juliana", "Fernanda", "Camila", "Amanda", "Beatriz", "Larissa",
	"Patricia", "Aline", "Bruna", "Carla", "Daniela", "Renata", "Vanessa", "Priscila",
	"Tatiane", "Cristina", "Adriana", "Luciana",
}

var ptBRLastNames = []string{
	"Silva", "Santos", "Oliveira", "Souza", "Rodrigues", "Ferreira", "Alves", "Pereira",
	"Lima", "Gomes", "Costa", "Ribeiro", "Martins", "Carvalho", "Almeida", "Lopes",
	"Soares", "Fernandes", "Vieira", "Barbosa", "Rocha", "Dias", "Nunes", "Moreira",
	"Nascimento", "Cardoso", "Correia", "Teixeira", "Araujo", "Melo",
}

// PtBRNameRule infers a person's first or last name from a pool of
// common Brazilian names, for a string field ending in FirstName or
// LastName. Names are ASCII (no diacritics) so a downstream rule built
// from them, like EmailRule lowercasing FirstName and LastName into an
// address, never has to decide whether to strip an accent.
type PtBRNameRule struct{}

// Priority runs PtBRNameRule ahead of NameRule (Priority 0), so it
// claims a matching field first when the locale is registered.
func (PtBRNameRule) Priority() int { return -1 }

// CanInfer matches a string field ending in FirstName or LastName, the
// same fields NameRule claims.
func (PtBRNameRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "FirstName", "LastName")
}

// Infer picks a first name, or a last name for a field ending in
// LastName, from ptBRFirstNames/ptBRLastNames.
func (PtBRNameRule) Infer(field autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	r := seed.Rand()
	if hasSuffix(field.Name, "LastName") {
		return ptBRLastNames[r.IntN(len(ptBRLastNames))]
	}
	return ptBRFirstNames[r.IntN(len(ptBRFirstNames))]
}

var ptBRCities = []string{
	"Sao Paulo", "Rio de Janeiro", "Belo Horizonte", "Brasilia", "Salvador",
	"Fortaleza", "Curitiba", "Recife", "Porto Alegre", "Manaus",
	"Belem", "Goiania", "Guarulhos", "Campinas", "Sao Luis",
	"Maceio", "Natal", "Teresina", "Florianopolis", "Joao Pessoa",
}

var ptBRStreetTypes = []string{"Rua", "Avenida", "Travessa", "Alameda"}

var ptBRStreetNames = []string{
	"das Flores", "Sete de Setembro", "Sao Joao", "Brasil", "Paulista",
	"Amazonas", "Rio Branco", "Getulio Vargas", "Santos Dumont", "Voluntarios da Patria",
	"Marechal Deodoro", "Barao do Rio Branco", "Tiradentes", "Independencia", "XV de Novembro",
}

// PtBRAddressRule infers a city or street name from a pool of real
// Brazilian cities and common street-name conventions, for a string
// field ending in Street, StreetName, Address or City.
type PtBRAddressRule struct{}

// Priority runs PtBRAddressRule ahead of AddressRule (Priority 0), so
// it claims a matching field first when the locale is registered.
func (PtBRAddressRule) Priority() int { return -1 }

// CanInfer matches the same fields AddressRule does: a string field
// ending in Street, StreetName, Address or City, except EmailAddress.
func (PtBRAddressRule) CanInfer(field autoseed.Field) bool {
	if hasSuffix(field.Name, "EmailAddress") {
		return false
	}
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "Street", "StreetName", "Address", "City")
}

// Infer picks a city for a field ending in City, or a "<street type>
// <street name>" pair (e.g. "Rua das Flores") otherwise.
func (PtBRAddressRule) Infer(field autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	r := seed.Rand()
	if hasSuffix(field.Name, "City") {
		return ptBRCities[r.IntN(len(ptBRCities))]
	}
	return fmt.Sprintf("%s %s", ptBRStreetTypes[r.IntN(len(ptBRStreetTypes))], ptBRStreetNames[r.IntN(len(ptBRStreetNames))])
}

// ptBRAreaCodes are real Brazilian DDD (area code) prefixes, spanning
// several states rather than just one metro area.
var ptBRAreaCodes = []string{
	"11", "21", "31", "41", "47", "48", "51", "61", "62", "71", "81", "85", "91",
}

// PtBRPhoneRule infers a Brazilian mobile phone number, for a string
// field ending in Phone or PhoneNumber.
type PtBRPhoneRule struct{}

// Priority runs PtBRPhoneRule ahead of PhoneRule (Priority 0), so it
// claims a matching field first when the locale is registered.
func (PtBRPhoneRule) Priority() int { return -1 }

// CanInfer matches a string field ending in Phone or PhoneNumber, the
// same fields PhoneRule claims.
func (PtBRPhoneRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "Phone", "PhoneNumber")
}

// Infer generates a Brazilian mobile number in
// "+55 (DDD) 9XXXX-XXXX" form, DDD drawn from ptBRAreaCodes.
func (PtBRPhoneRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	r := seed.Rand()
	area := ptBRAreaCodes[r.IntN(len(ptBRAreaCodes))]
	return fmt.Sprintf("+55 (%s) 9%04d-%04d", area, r.IntN(10000), r.IntN(10000))
}
