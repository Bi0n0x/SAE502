package games

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

var ErrUnknownGame = errors.New("unknown game")

var (
	mu       sync.RWMutex
	registry = map[string]Game{}
	order    []string
)

func Register(g Game) {
	mu.Lock()
	defer mu.Unlock()
	key := g.Key()
	if key == "" {
		panic("games: empty game key")
	}
	if _, exists := registry[key]; exists {
		panic(fmt.Sprintf("games: game %q already registered", key))
	}
	registry[key] = g
	order = append(order, key)
	sort.Strings(order)
}

func Get(key string) (Game, error) {
	mu.RLock()
	defer mu.RUnlock()
	g, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownGame, key)
	}
	return g, nil
}

func All() []Game {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Game, 0, len(order))
	for _, k := range order {
		out = append(out, registry[k])
	}
	return out
}

func Describe(g Game) map[string]any {
	d := map[string]any{
		"key":         g.Key(),
		"name":        g.Name(),
		"description": g.Description(),
		"config":      g.Config(),
	}
	if m, ok := g.(MinBetter); ok {
		d["min_bet"] = m.MinBet()
	}
	if m, ok := g.(MaxBetter); ok {
		d["max_bet"] = m.MaxBet()
	}
	return d
}
