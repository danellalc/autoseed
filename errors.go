package autoseed

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownReference is returned when an Entity's Reference names a Target
// that does not match any Entity.Name in the model.
var ErrUnknownReference = errors.New("autoseed: reference targets an unknown entity")

// ErrUnsatisfiableCycle is returned when a group of entities form a foreign
// key cycle with no nullable reference to break it: no insertion order can
// satisfy every required foreign key at once.
var ErrUnsatisfiableCycle = errors.New("autoseed: unsatisfiable required foreign key cycle")

func unknownReferenceError(entity string, fields []string, target string) error {
	return fmt.Errorf("%w: %s.%s references %q", ErrUnknownReference, entity, strings.Join(fields, "+"), target)
}

func unsatisfiableCycleError(entities []string) error {
	return fmt.Errorf("%w: %s", ErrUnsatisfiableCycle, strings.Join(entities, " -> "))
}
