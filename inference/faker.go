package inference

import (
	"math/rand/v2"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/danellalc/autoseed"
)

func faker(seed *autoseed.SeededSource) *gofakeit.Faker {
	return gofakeit.NewFaker(rand.NewPCG(seed.Seed(), seed.Seed()), false)
}
