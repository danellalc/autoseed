package autoseed

import (
	"encoding/binary"
	"hash/fnv"
	"math/rand/v2"
	"strconv"
)

// SeededSource derives deterministic randomness for one position in the
// generation tree — root, entity, row index, field — so that generating
// row 500 of an entity in isolation produces exactly the value it would
// produce inside a full batch. Every autoseed.Rand call anywhere in the
// pipeline must come from a SeededSource, never from math/rand directly.
type SeededSource struct {
	seed uint64
}

// NewSeededSource returns the root SeededSource for a seed.
func NewSeededSource(seed uint64) *SeededSource {
	return &SeededSource{seed: seed}
}

// Entity returns the SeededSource scoped to one entity.
func (s *SeededSource) Entity(name string) *SeededSource {
	return &SeededSource{seed: mixString(s.seed, name)}
}

// Row returns the SeededSource scoped to one row index within an entity.
func (s *SeededSource) Row(index int) *SeededSource {
	return &SeededSource{seed: mixInt(s.seed, index)}
}

// Field returns the SeededSource scoped to one field within a row.
func (s *SeededSource) Field(name string) *SeededSource {
	return &SeededSource{seed: mixString(s.seed, name)}
}

// Rand returns the deterministic *rand.Rand for the current position.
func (s *SeededSource) Rand() *rand.Rand {
	return rand.New(rand.NewPCG(s.seed, s.seed))
}

func mixString(seed uint64, part string) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], seed)
	h.Write(buf[:])
	h.Write([]byte(part))
	return h.Sum64()
}

func mixInt(seed uint64, part int) uint64 {
	return mixString(seed, strconv.Itoa(part))
}
