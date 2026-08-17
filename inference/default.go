package inference

// NewDefaultGenerator returns a Generator with every built-in rule.
func NewDefaultGenerator() *Generator {
	return NewGenerator(
		DocumentRule{},
		NameRule{},
		EmailRule{},
		PhoneRule{},
		URLRule{},
		AddressRule{},
		CreatedAtRule{},
		UpdatedAtRule{},
		PriceRule{},
		QuantityRule{},
		CorrelatedTotalRule{},
		SoftDeleteRule{},
		genericTextRule{},
		genericNumberRule{},
		genericBoolRule{},
		genericTimeRule{},
	)
}
