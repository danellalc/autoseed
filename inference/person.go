package inference

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/danellalc/autoseed"
)

// NameRule infers a person's first or last name for a string field ending
// in FirstName or LastName.
type NameRule struct{}

// Priority runs NameRule before EmailRule, which reads its output back.
func (NameRule) Priority() int { return 0 }

// CanInfer matches a string field ending in FirstName or LastName.
func (NameRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "FirstName", "LastName")
}

// Infer generates a first name, or a last name for a field ending in
// LastName.
func (NameRule) Infer(field autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	f := faker(seed)
	if hasSuffix(field.Name, "LastName") {
		return f.LastName()
	}
	return f.FirstName()
}

// EmailRule infers an email address for a string field ending in Email or
// EmailAddress. When FirstName and LastName were already generated on the
// same row, the address is built from them so the two agree.
type EmailRule struct{}

// Priority runs EmailRule after NameRule, so a same-row FirstName and
// LastName are already generated when Infer looks for them.
func (EmailRule) Priority() int { return 1 }

// CanInfer matches a string field ending in Email or EmailAddress.
func (EmailRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "Email", "EmailAddress")
}

// Infer builds firstname.lastname@domain from a same-row FirstName and
// LastName when both were already generated, otherwise draws an
// independent address.
func (EmailRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, generated map[string]any) any {
	f := faker(seed)

	first, hasFirst := findSibling[string](generated, "FirstName")
	last, hasLast := findSibling[string](generated, "LastName")
	if hasFirst && hasLast {
		return fmt.Sprintf("%s.%s@%s", strings.ToLower(first), strings.ToLower(last), f.DomainName())
	}
	return f.Email()
}

// PhoneRule infers a phone number for a string field ending in Phone or
// PhoneNumber.
type PhoneRule struct{}

// Priority runs PhoneRule before generic string rules.
func (PhoneRule) Priority() int { return 0 }

// CanInfer matches a string field ending in Phone or PhoneNumber.
func (PhoneRule) CanInfer(field autoseed.Field) bool {
	return isKind(field.Type, reflect.String) && hasSuffix(field.Name, "Phone", "PhoneNumber")
}

// Infer generates a phone number.
func (PhoneRule) Infer(_ autoseed.Field, seed *autoseed.SeededSource, _ map[string]any) any {
	return faker(seed).Phone()
}
