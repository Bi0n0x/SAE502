package catalog

import (
	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/games/roulette"
	"github.com/casino-rt/backend/internal/games/slots"
)

func Register() {
	games.Register(roulette.New())
	for _, def := range slots.BuiltIn() {
		games.Register(slots.MustCompile(def))
	}
}
