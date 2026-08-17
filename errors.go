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

// ErrDuplicateEntity is returned when two or more Entity values in a model
// share the same Name: a ModelSource bug, since the graph cannot tell them
// apart.
var ErrDuplicateEntity = errors.New("autoseed: duplicate entity name")

// ErrInvalidReference is returned when a Reference names no Fields: there
// is no column for it to mean anything.
var ErrInvalidReference = errors.New("autoseed: reference names no fields")

// ErrUnsupportedField is returned when no inference rule can produce a
// value for a field: an adapter read a construct value generation does
// not understand yet, named rather than silently generating the wrong
// thing or the zero value.
var ErrUnsupportedField = errors.New("autoseed: no inference rule for field")

// ErrUnsatisfiableUniqueness is returned when a duplicate value for a
// unique field could not be fixed within the retry budget: the value
// space is too small for the requested row count.
var ErrUnsatisfiableUniqueness = errors.New("autoseed: could not generate a unique value")

func unknownReferenceError(entity string, fields []string, target string) error {
	return fmt.Errorf("%w: %s.%s references %q", ErrUnknownReference, entity, strings.Join(fields, "+"), target)
}

func unsatisfiableCycleError(cycle []string) error {
	return fmt.Errorf("%w: %s", ErrUnsatisfiableCycle, strings.Join(cycle, " -> "))
}

func duplicateEntityError(name string) error {
	return fmt.Errorf("%w: %q", ErrDuplicateEntity, name)
}

func invalidReferenceError(entity, target string) error {
	return fmt.Errorf("%w: %s references %q with no fields named", ErrInvalidReference, entity, target)
}

func unsatisfiableUniquenessError(entity, field string, attempts int) error {
	return fmt.Errorf("%w: %s.%s after %d attempts", ErrUnsatisfiableUniqueness, entity, field, attempts)
}
