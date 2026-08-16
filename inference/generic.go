package inference

import (
	"reflect"
	"strings"

	"github.com/danellalc/autoseed"
)

const genericTextWordCount = 3

type genericTextRule struct{}

func (genericTextRule) Priority() int { return 10 }

func (genericTextRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String)
}

func (genericTextRule) Infer(field autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	text := faker(seed).LoremIpsumSentence(genericTextWordCount)
	text = strings.TrimSuffix(text, ".")
	if field.Size > 0 && len(text) > field.Size {
		text = text[:field.Size]
	}
	return text
}

const (
	minGenericNumber = 0
	maxGenericNumber = 1000
)

type genericNumberRule struct{}

func (genericNumberRule) Priority() int { return 10 }

func (genericNumberRule) CanInfer(field autoseed.Field) bool {
	return isInt(field.Type) || isFloat(field.Type)
}

func (genericNumberRule) Infer(field autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	if isFloat(field.Type) {
		return faker(seed).Float64Range(minGenericNumber, maxGenericNumber)
	}
	return faker(seed).Number(minGenericNumber, maxGenericNumber)
}

type genericBoolRule struct{}

func (genericBoolRule) Priority() int { return 10 }

func (genericBoolRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.Bool)
}

func (genericBoolRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return faker(seed).Bool()
}
