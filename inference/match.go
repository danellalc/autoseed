package inference

import (
	"reflect"
	"strings"
	"time"

	"github.com/danellalc/autoseed"
)

var timeType = reflect.TypeOf(time.Time{})

func hasSuffix(name string, suffixes ...string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range suffixes {
		if strings.HasSuffix(lower, strings.ToLower(suffix)) {
			return true
		}
	}
	return false
}

func isKind(t reflect.Type, kinds ...reflect.Kind) bool {
	if t == nil {
		return false
	}
	for _, kind := range kinds {
		if t.Kind() == kind {
			return true
		}
	}
	return false
}

func isTime(t reflect.Type) bool {
	return t == timeType
}

var (
	intKinds = []reflect.Kind{
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
	}
	floatKinds = []reflect.Kind{reflect.Float32, reflect.Float64}
)

func isInt(t reflect.Type) bool {
	return isKind(t, intKinds...)
}

func isFloat(t reflect.Type) bool {
	return isKind(t, floatKinds...)
}

func truncate(field autoseed.Field, value any) any {
	if field.Size <= 0 {
		return value
	}
	text, ok := value.(string)
	if !ok || len(text) <= field.Size {
		return value
	}
	return text[:field.Size]
}
