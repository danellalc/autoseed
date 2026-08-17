package autoseed

// Options configures Seed and SeedCoverage. Build one with NewOptions and
// functional Option values; the zero value is not meaningful on its own.
type Options struct {
	Seed  uint64
	Scale int
}

// NewOptions returns the default Options with every given Option applied.
// The default seed is 0; the default scale is 100.
func NewOptions(opts ...Option) Options {
	options := Options{Seed: 0, Scale: 100}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

// Option configures Options.
type Option func(*Options)

// WithSeed fixes the root seed every random draw derives from. Same seed,
// same data, always.
func WithSeed(seed uint64) Option {
	return func(o *Options) { o.Seed = seed }
}

// WithScale sets the row count for root entities — those with no required
// foreign key into another seeded entity. Related entities scale from
// their principal's row count instead of Scale directly.
func WithScale(scale int) Option {
	return func(o *Options) { o.Scale = scale }
}
