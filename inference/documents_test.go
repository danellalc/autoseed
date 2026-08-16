package inference_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

func inferField(t *testing.T, rule inference.Rule, field autoseed.Field, seed uint64) any {
	t.Helper()
	source := autoseed.NewSeededSource(seed).Entity("E").Row(0).Field(field.Name)
	return rule.Infer(field, source, nil)
}

func TestDocumentRule_CPFHasValidCheckDigits(t *testing.T) {
	field := autoseed.Field{Name: "CPF", Type: reflect.TypeOf("")}
	for seed := uint64(0); seed < 20; seed++ {
		value := inferField(t, inference.DocumentRule{}, field, seed).(string)
		if len(value) != 11 {
			t.Fatalf("seed %d: CPF = %q, want 11 digits", seed, value)
		}
		if !validCPF(value) {
			t.Fatalf("seed %d: CPF = %q has an invalid check digit", seed, value)
		}
	}
}

func TestDocumentRule_CNPJHasValidCheckDigits(t *testing.T) {
	field := autoseed.Field{Name: "CNPJ", Type: reflect.TypeOf("")}
	for seed := uint64(0); seed < 20; seed++ {
		value := inferField(t, inference.DocumentRule{}, field, seed).(string)
		if len(value) != 14 {
			t.Fatalf("seed %d: CNPJ = %q, want 14 digits", seed, value)
		}
		if !validCNPJ(value) {
			t.Fatalf("seed %d: CNPJ = %q has an invalid check digit", seed, value)
		}
	}
}

func TestDocumentRule_CanInfer(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want bool
	}{
		{"CPF", reflect.TypeOf(""), true},
		{"OwnerCPF", reflect.TypeOf(""), true},
		{"CNPJ", reflect.TypeOf(""), true},
		{"CPF", reflect.TypeOf(0), false},
		{"Name", reflect.TypeOf(""), false},
	}
	for _, tt := range tests {
		field := autoseed.Field{Name: tt.name, Type: tt.typ}
		if got := (inference.DocumentRule{}).CanInfer(field); got != tt.want {
			t.Errorf("CanInfer(%s %s) = %v, want %v", tt.name, tt.typ, got, tt.want)
		}
	}
}

// validCPF and validCNPJ independently recompute the check digits using
// the digit-weighted-sum algorithm (mod 11), rather than reusing the
// package's own implementation, so this test can't pass by construction.
func validCPF(cpf string) bool {
	if len(cpf) != 11 {
		return false
	}
	digits := toDigits(cpf)
	first := checkDigitFrom(digits[:9], 10)
	second := checkDigitFrom(digits[:10], 11)
	return digits[9] == first && digits[10] == second
}

func validCNPJ(cnpj string) bool {
	if len(cnpj) != 14 {
		return false
	}
	digits := toDigits(cnpj)
	firstWeights := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	secondWeights := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	first := checkDigitWeights(digits[:12], firstWeights)
	second := checkDigitWeights(digits[:13], secondWeights)
	return digits[12] == first && digits[13] == second
}

func toDigits(s string) []int {
	digits := make([]int, len(s))
	for i, r := range s {
		digits[i], _ = strconv.Atoi(string(r))
	}
	return digits
}

func checkDigitFrom(digits []int, firstWeight int) int {
	sum, weight := 0, firstWeight
	for _, d := range digits {
		sum += d * weight
		weight--
	}
	return toCheckDigit(sum)
}

func checkDigitWeights(digits, weights []int) int {
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
