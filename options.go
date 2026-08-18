package autoseed

// Options configures Seed (and, once built, the planned SeedCoverage).
// Build one with NewOptions and functional Option values; the zero value
// is not meaningful on its own.
type Options struct {
	Seed    uint64
	Scale   int
	NilRate float64
	Locale  string
}

// NewOptions returns the default Options with every given Option applied.
// The default seed is 0; the default scale is 100; the default nil rate
// is 0 (every nullable field always gets a generated value); the default
// locale is "" (generic, English-shaped values).
func NewOptions(opts ...Option) Options {
	options := Options{Seed: 0, Scale: 100, NilRate: 0, Locale: ""}
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

// WithNilRate sets the per-row probability that a Nullable field's value
// is left out entirely instead of generated — a field the adapter marks
// Nullable only when persistence can actually turn a missing value into
// a real database NULL. rate is clamped to [0,1].
func WithNilRate(rate float64) Option {
	return func(o *Options) {
		if rate < 0 {
			rate = 0
		}
		if rate > 1 {
			rate = 1
		}
		o.NilRate = rate
	}
}

// WithLocale sets which locale's own rules claim a name, address and
// phone field before the generic, English-shaped fallback gets a turn —
// "pt_BR" today, more as demand shows up, the same on-demand bar every
// adapter and rule in this project ships under. An unrecognized locale,
// including the empty default, falls back to the generic rules — the
// same as not setting a locale at all, never an error, matching every
// other Option's own silent-normalize behavior (WithNilRate clamps
// rather than rejecting an out-of-range rate).
func WithLocale(locale string) Option {
	return func(o *Options) { o.Locale = locale }
}
