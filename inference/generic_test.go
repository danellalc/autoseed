package inference_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/danellalc/autoseed"
	"github.com/danellalc/autoseed/inference"
)

func TestDefaultGenerator_GenericTextRespectsSize(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Widget",
		Fields: []autoseed.Field{
			{Name: "Nickname", Type: reflect.TypeOf(""), Size: 5},
		},
	}

	gen := inference.NewDefaultGenerator()
	for seed := uint64(0); seed < 20; seed++ {
		values, err := gen.GenerateRow(entity, autoseed.NewSeededSource(seed))
		if err != nil {
			t.Fatalf("seed %d: GenerateRow: %v", seed, err)
		}
		value := values["Nickname"].(string)
		if len(value) > 5 {
			t.Fatalf("seed %d: Nickname = %q has length %d, want at most 5", seed, value, len(value))
		}
	}
}

func TestDefaultGenerator_ClaimsEveryBasicKind(t *testing.T) {
	entity := autoseed.Entity{
		Name: "Widget",
		Fields: []autoseed.Field{
			{Name: "Description", Type: reflect.TypeOf("")},
			{Name: "Count", Type: reflect.TypeOf(0)},
			{Name: "Score", Type: reflect.TypeOf(float64(0))},
			{Name: "Active", Type: reflect.TypeOf(false)},
			{Name: "SeenAt", Type: reflect.TypeOf(time.Time{})},
		},
	}

	values, err := inference.NewDefaultGenerator().GenerateRow(entity, autoseed.NewSeededSource(1))
	if err != nil {
		t.Fatalf("GenerateRow: %v", err)
	}
	for _, field := range entity.Fields {
		if _, ok := values[field.Name]; !ok {
			t.Fatalf("field %q was not claimed by any rule", field.Name)
		}
	}
}
