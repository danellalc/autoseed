package inference

import (
	"database/sql"
	"reflect"
)

// nullableWrapperTypes are the database/sql "nullable primitive" types: a
// struct wrapping one typed value field (always index 0) and a Valid
// bool. GenerateRow unwraps a field of one of these types to its value
// field's type for rule matching — so a field named Email of type
// sql.NullString still gets EmailRule's treatment, not just generic
// text — then rewraps the result with Valid true.
var nullableWrapperTypes = map[reflect.Type]bool{
	reflect.TypeOf(sql.NullString{}):  true,
	reflect.TypeOf(sql.NullInt64{}):   true,
	reflect.TypeOf(sql.NullInt32{}):   true,
	reflect.TypeOf(sql.NullInt16{}):   true,
	reflect.TypeOf(sql.NullByte{}):    true,
	reflect.TypeOf(sql.NullBool{}):    true,
	reflect.TypeOf(sql.NullFloat64{}): true,
	reflect.TypeOf(sql.NullTime{}):    true,
}

func isNullableWrapper(t reflect.Type) bool {
	return t != nil && nullableWrapperTypes[t]
}

func unwrapNullable(t reflect.Type) reflect.Type {
	return t.Field(0).Type
}

// wrapNullable builds a value of nullableType (one of the database/sql
// Null* types) holding value with Valid set true. value must be
// convertible to the wrapper's value field type — true for anything an
// inference.Rule matched against the corresponding unwrapped Field could
// plausibly return (int/int64/float64 for a numeric field, string,
// bool, time.Time).
func wrapNullable(nullableType reflect.Type, value any) any {
	wrapper := reflect.New(nullableType).Elem()
	converted := reflect.ValueOf(value).Convert(wrapper.Field(0).Type())
	wrapper.Field(0).Set(converted)
	wrapper.FieldByName("Valid").SetBool(true)
	return wrapper.Interface()
}
