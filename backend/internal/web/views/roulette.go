package views

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/casino-rt/backend/internal/games/roulette"
)

// wheelOrder is the pocket order of a European wheel, clockwise from zero.
var wheelOrder = []int{
	0, 32, 15, 19, 4, 21, 2, 25, 17, 34, 6, 27, 13, 36, 11, 30, 8, 23, 10,
	5, 24, 16, 33, 1, 20, 14, 31, 9, 22, 18, 29, 7, 28, 12, 35, 3, 26,
}

const spinSeconds = 4.5

type RouletteGame struct {
	Key         string
	Name        string
	Description string
	MinBet      int64
	MaxBet      int64
	Kinds       map[string]roulette.BetKind
}

type RouletteSpin struct {
	Result int
	Color  string
	Won    bool
	Bet    int64
	Payout int64
	Option string
}

type RouletteStage struct {
	Game   *RouletteGame
	Spin   *RouletteSpin
	Amount int64
	Option string
	Token  string
	Error  string
}

func NewRouletteGame(key, name, description string, minBet, maxBet int64) *RouletteGame {
	g := &RouletteGame{
		Key:         key,
		Name:        name,
		Description: description,
		MinBet:      minBet,
		MaxBet:      maxBet,
		Kinds:       map[string]roulette.BetKind{},
	}
	for _, k := range roulette.BetKinds() {
		g.Kinds[k.ID] = k
	}
	return g
}

func RouletteIdle(g *RouletteGame, amount int64, option, token string) RouletteStage {
	return RouletteStage{Game: g, Amount: amount, Option: option, Token: token}
}

func RouletteResult(g *RouletteGame, detail map[string]any, bet, payout int64, option string, amount int64, token string) RouletteStage {
	var d struct {
		Result int    `json:"result"`
		Color  string `json:"color"`
		Won    bool   `json:"won"`
	}
	raw, _ := json.Marshal(detail)
	if err := json.Unmarshal(raw, &d); err != nil {
		return RouletteIdle(g, amount, option, token)
	}
	st := RouletteIdle(g, amount, option, token)
	st.Spin = &RouletteSpin{
		Result: d.Result,
		Color:  d.Color,
		Won:    d.Won,
		Bet:    bet,
		Payout: payout,
		Option: g.OptionLabel(option),
	}
	return st
}

func (g *RouletteGame) OptionLabel(option string) string {
	id, n, _ := strings.Cut(option, ":")
	k, ok := g.Kinds[id]
	if !ok {
		return option
	}
	if k.NeedsNumber {
		return k.Label + " " + n
	}
	return k.Label
}

// turn is the wheel rotation that brings the result under the pointer at the
// top, after a few full turns.
func (s *RouletteSpin) turn() float64 {
	step := 360.0 / float64(len(wheelOrder))
	for i, n := range wheelOrder {
		if n == s.Result {
			return 5*360 - (float64(i)+0.5)*step
		}
	}
	return 0
}

func pocketColor(n int) string { return roulette.Color(n) }

type boardCell struct {
	Value string
	Label string
	Class string
	// Grid areas of the cell: the wide table on desktop, and the same table
	// turned upright on narrow screens.
	Wide string
	Tall string
}

func area(row, col, rows, cols int) string {
	return fmt.Sprintf("%d / %d / span %d / span %d", row, col, rows, cols)
}

// boardRows lays out the felt like a real table: zero at the head, the numbers
// in three rows, column bets at the end, then dozens and even-money bets.
func boardRows() []boardCell {
	cells := []boardCell{{
		Value: "number:0", Label: "0", Class: "green zero",
		Wide: area(1, 1, 3, 1), Tall: area(1, 3, 1, 3),
	}}
	for n := 1; n <= 36; n++ {
		line, pos := (n-1)/3, (n-1)%3
		cells = append(cells, boardCell{
			Value: "number:" + strconv.Itoa(n),
			Label: strconv.Itoa(n),
			Class: roulette.Color(n),
			Wide:  area(3-pos, line+2, 1, 1),
			Tall:  area(line+2, pos+3, 1, 1),
		})
	}
	for k := 1; k <= 3; k++ {
		cells = append(cells, boardCell{
			Value: "column" + strconv.Itoa(k), Label: "2:1", Class: "outside",
			Wide: area(4-k, 14, 1, 1), Tall: area(14, k+2, 1, 1),
		})
	}
	for i, label := range []string{"1-12", "13-24", "25-36"} {
		cells = append(cells, boardCell{
			Value: "dozen" + strconv.Itoa(i+1), Label: label, Class: "outside side",
			Wide: area(4, 2+4*i, 1, 4), Tall: area(2+4*i, 2, 4, 1),
		})
	}
	for j, c := range []boardCell{
		{Value: "low", Label: "1-18"},
		{Value: "even", Label: "Pair"},
		{Value: "red", Label: "Rouge", Class: "red"},
		{Value: "black", Label: "Noir", Class: "black"},
		{Value: "odd", Label: "Impair"},
		{Value: "high", Label: "19-36"},
	} {
		c.Class += " outside side"
		c.Wide, c.Tall = area(5, 2+2*j, 1, 2), area(2+2*j, 1, 2, 1)
		cells = append(cells, c)
	}
	return cells
}

var board = boardRows()

func (st RouletteStage) Settle() float64 { return spinSeconds }
