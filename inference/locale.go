package inference

// localeRules returns the rules that claim a name, address or phone
// field ahead of the generic ones for locale, or nil for an
// unrecognized locale.
func localeRules(locale string) []Rule {
	switch locale {
	case "pt_BR":
		return []Rule{PtBRNameRule{}, PtBRAddressRule{}, PtBRPhoneRule{}}
	default:
		return nil
	}
}
