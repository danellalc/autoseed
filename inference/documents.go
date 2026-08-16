package inference

import (
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"

	"github.com/danellalc/autoseed"
)

var (
	cnpjFirstCheckWeights  = []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	cnpjSecondCheckWeights = []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
)

// DocumentRule infers a Brazilian CPF or CNPJ, digits only, with a valid
// check digit, for a string field named Cpf or Cnpj.
type DocumentRule struct{}

// Priority runs DocumentRule before generic string rules.
func (DocumentRule) Priority() int { return 0 }

// CanInfer matches a string field ending in Cpf or Cnpj.
func (DocumentRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "Cpf", "Cnpj")
}

// Infer generates digits-only, with a valid check digit: 11 digits for a
// Cpf field, 14 for a Cnpj field.
func (DocumentRule) Infer(field autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	r := seed.Rand()
	if hasSuffix(field.Name, "Cnpj") {
		return cnpj(r)
	}
	return cpf(r)
}

func cpf(r *rand.Rand) string {
	digits := randomDigits(r, 9)
	first := checkDigit(digits, 10)
	withFirst := append(append([]int(nil), digits...), first)
	second := checkDigit(withFirst, 11)
	return joinDigits(append(withFirst, second))
}

func cnpj(r *rand.Rand) string {
	digits := randomDigits(r, 12)
	first := checkDigitWeighted(digits, cnpjFirstCheckWeights)
	withFirst := append(append([]int(nil), digits...), first)
	second := checkDigitWeighted(withFirst, cnpjSecondCheckWeights)
	return joinDigits(append(withFirst, second))
}

func randomDigits(r *rand.Rand, n int) []int {
	digits := make([]int, n)
	for i := range digits {
		digits[i] = r.IntN(10)
	}
	return digits
}

func checkDigit(digits []int, firstWeight int) int {
	sum := 0
	weight := firstWeight
	for _, d := range digits {
		sum += d * weight
		weight--
	}
	return toCheckDigit(sum)
}

func checkDigitWeighted(digits, weights []int) int {
	sum := 0
	for i, d := range digits {
		sum += d * weights[i]
	}
	return toCheckDigit(sum)
}

func toCheckDigit(sum int) int {
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

func joinDigits(digits []int) string {
	var b strings.Builder
	for _, d := range digits {
		b.WriteString(strconv.Itoa(d))
	}
	return b.String()
}
