package games

import "github.com/casino-rt/backend/internal/rng"

type Bet struct {
	Amount int64

	Option string

	Params map[string]any
}

type Outcome struct {
	Payout int64

	Label string

	Detail map[string]any
}

type Game interface {
	Key() string
	Name() string
	Description() string

	Config() map[string]any

	ValidateBet(b Bet) error

	Play(r rng.Source, b Bet) (Outcome, error)
}

type MinBetter interface{ MinBet() int64 }
type MaxBetter interface{ MaxBet() int64 }
