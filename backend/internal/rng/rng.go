package rng

import (
	crand "crypto/rand"
	"math/big"
	mrand "math/rand"
)

type Source interface {
	Intn(n int) int
}

type Crypto struct{}

func (Crypto) Intn(n int) int {
	if n <= 0 {
		panic("rng: Intn called with n <= 0")
	}
	v, err := crand.Int(crand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic("rng: entropy source unavailable: " + err.Error())
	}
	return int(v.Int64())
}

type Seeded struct{ r *mrand.Rand }

func NewSeeded(seed int64) *Seeded {
	return &Seeded{r: mrand.New(mrand.NewSource(seed))}
}

func (s *Seeded) Intn(n int) int { return s.r.Intn(n) }
