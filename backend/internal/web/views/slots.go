package views

import (
	"cmp"
	"encoding/json"
	"slices"

	"github.com/casino-rt/backend/internal/games/slots"
)

type SlotsGame struct {
	Key            string
	Name           string
	Description    string
	MinBet         int64
	MaxBet         int64
	Rows           int
	Reels          [][]string
	Paylines       [][]int
	Symbols        map[string]slots.Symbol
	Pays           []PayRow
	Scatter        *slots.Symbol
	ScatterPays    []Pay
	WildMultiplier int64
	Bg             string
	Accent         string
	Win            string
	Speed          int
	RTP            float64
	Volatility     string
}

type PayRow struct {
	Symbol slots.Symbol
	Pays   []Pay
}

type Pay struct {
	Count    int
	PerMille int64
}

type Cell struct {
	Symbol slots.Symbol
	Hit    bool
}

type LineWin struct {
	Line     int
	Symbol   slots.Symbol
	Count    int
	WithWild bool
	Payout   int64
}

type SlotsSpin struct {
	Bet        int64
	Payout     int64
	Label      string
	Lines      []LineWin
	Scatters   int
	ScatterWin int64
}

type SlotsStage struct {
	Game *SlotsGame
	// Reels[r] is the strip shown in reel r, top to bottom. The first Rows
	// cells are where the reel stops; the rest scroll past during a spin.
	Reels  [][]Cell
	Spin   *SlotsSpin
	Amount int64
	Token  string
	Error  string
}

func NewSlotsGame(d slots.Definition, minBet, maxBet int64) *SlotsGame {
	g := &SlotsGame{
		Key:            d.Key,
		Name:           d.Name,
		Description:    d.Description,
		MinBet:         minBet,
		MaxBet:         maxBet,
		Rows:           d.Rows,
		Reels:          d.Reels,
		Paylines:       d.Paylines,
		Symbols:        make(map[string]slots.Symbol, len(d.Symbols)),
		WildMultiplier: d.WildMultiplier,
		Bg:             "#120d2b",
		Accent:         "#bb6bd9",
		Win:            "#f2c94c",
		Speed:          90,
		RTP:            d.TheoreticalRTP,
		Volatility:     d.Volatility,
	}
	if palette, ok := d.Theme["palette"].(map[string]string); ok {
		g.Bg = cmp.Or(palette["bg"], g.Bg)
		g.Accent = cmp.Or(palette["accent"], g.Accent)
		g.Win = cmp.Or(palette["win"], g.Win)
	}
	if speed, ok := d.Theme["reel_speed"].(int); ok && speed > 0 {
		g.Speed = speed
	}

	for _, s := range d.Symbols {
		g.Symbols[s.ID] = s
		if s.Scatter {
			sym := s
			g.Scatter = &sym
			continue
		}
		g.Pays = append(g.Pays, PayRow{Symbol: s, Pays: pays(d.Paytable[s.ID])})
	}
	slices.Reverse(g.Pays)
	g.ScatterPays = pays(d.ScatterPays)
	return g
}

func pays(table map[int]int64) []Pay {
	out := make([]Pay, 0, len(table))
	for count, perMille := range table {
		out = append(out, Pay{Count: count, PerMille: perMille})
	}
	slices.SortFunc(out, func(a, b Pay) int { return a.Count - b.Count })
	return out
}

// reelDuration spreads the stops so the reels land one after the other, left
// to right. reel_speed from the machine theme scales the whole spin.
func (g *SlotsGame) reelDuration(reel int) float64 {
	return (0.8 + 0.3*float64(reel)) * 90 / float64(g.Speed)
}

// Settle is the time the last reel needs to stop.
func (g *SlotsGame) Settle() float64 { return g.reelDuration(len(g.Reels)-1) + 0.1 }

func SlotsIdle(g *SlotsGame, amount int64, token string) SlotsStage {
	stops := make([]int, len(g.Reels))
	for r := range stops {
		stops[r] = r * 7
	}
	return SlotsStage{Game: g, Reels: g.strips(stops, 0), Amount: amount, Token: token}
}

func SlotsResult(g *SlotsGame, detail map[string]any, bet, payout int64, label string, amount int64, token string) SlotsStage {
	var d struct {
		Stops        []int           `json:"stops"`
		LineWins     []slots.LineWin `json:"line_wins"`
		ScatterCount int             `json:"scatter_count"`
		ScatterWin   int64           `json:"scatter_win"`
	}
	raw, _ := json.Marshal(detail)
	if err := json.Unmarshal(raw, &d); err != nil || len(d.Stops) != len(g.Reels) {
		return SlotsIdle(g, amount, token)
	}

	st := SlotsStage{Game: g, Reels: g.strips(d.Stops, 10), Amount: amount, Token: token}
	spin := &SlotsSpin{
		Bet:        bet,
		Payout:     payout,
		Label:      label,
		Scatters:   d.ScatterCount,
		ScatterWin: d.ScatterWin,
	}
	for _, w := range d.LineWins {
		spin.Lines = append(spin.Lines, LineWin{
			Line:     w.Line + 1,
			Symbol:   g.Symbols[w.Symbol],
			Count:    w.Count,
			WithWild: w.WithWild,
			Payout:   w.Payout,
		})
		for _, pos := range w.Positions {
			if len(pos) == 2 && pos[1] < len(st.Reels) && pos[0] < g.Rows {
				st.Reels[pos[1]][pos[0]].Hit = true
			}
		}
	}
	if d.ScatterWin > 0 && g.Scatter != nil {
		for _, reel := range st.Reels {
			for row := 0; row < g.Rows; row++ {
				if reel[row].Symbol.ID == g.Scatter.ID {
					reel[row].Hit = true
				}
			}
		}
	}
	st.Spin = spin
	return st
}

// strips builds each reel from the real strip: the stop position first, then
// the symbols that follow it, which scroll by before the reel lands. Later
// reels get a longer run so they keep spinning after the first ones stop.
func (g *SlotsGame) strips(stops []int, lead int) [][]Cell {
	out := make([][]Cell, len(g.Reels))
	for r, strip := range g.Reels {
		n := g.Rows
		if lead > 0 {
			n += lead + 3*r
		}
		cells := make([]Cell, n)
		for i := range cells {
			cells[i] = Cell{Symbol: g.Symbols[strip[(stops[r]+i)%len(strip)]]}
		}
		out[r] = cells
	}
	return out
}

func (st SlotsStage) Settle() float64 { return st.Game.Settle() }
