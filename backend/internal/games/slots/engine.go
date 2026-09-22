package slots

import (
	"fmt"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/rng"
)

type Symbol struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
	Color string `json:"color"`

	Wild bool `json:"wild"`

	Scatter bool `json:"scatter"`
}

type Definition struct {
	Key         string
	Name        string
	Description string

	Rows int

	Reels [][]string

	Paylines [][]int

	Symbols []Symbol

	Paytable map[string]map[int]int64

	ScatterPays map[int]int64

	WildMultiplier int64

	Theme map[string]any

	MinBetCredits int64
	MaxBetCredits int64

	TheoreticalRTP float64
	Volatility     string
}

type Machine struct {
	def     Definition
	symbols map[string]Symbol
	scatter string
	wilds   map[string]bool
}

func Compile(def Definition) (*Machine, error) {
	if def.Key == "" {
		return nil, fmt.Errorf("slots: cle manquante")
	}
	if def.Rows < 1 {
		return nil, fmt.Errorf("slots[%s]: Rows doit etre >= 1", def.Key)
	}
	if len(def.Reels) < 3 {
		return nil, fmt.Errorf("slots[%s]: au moins 3 rouleaux sont necessaires", def.Key)
	}

	m := &Machine{
		def:     def,
		symbols: make(map[string]Symbol, len(def.Symbols)),
		wilds:   make(map[string]bool),
	}
	for _, s := range def.Symbols {
		if _, dup := m.symbols[s.ID]; dup {
			return nil, fmt.Errorf("slots[%s]: symbole %q declare deux fois", def.Key, s.ID)
		}
		if s.Wild && s.Scatter {
			return nil, fmt.Errorf("slots[%s]: %q ne peut pas etre wild et scatter", def.Key, s.ID)
		}
		m.symbols[s.ID] = s
		if s.Wild {
			m.wilds[s.ID] = true
		}
		if s.Scatter {
			if m.scatter != "" {
				return nil, fmt.Errorf("slots[%s]: un seul scatter est supporte", def.Key)
			}
			m.scatter = s.ID
		}
	}

	for i, strip := range def.Reels {
		if len(strip) < def.Rows {
			return nil, fmt.Errorf("slots[%s]: rouleau %d plus court que le nombre de lignes", def.Key, i)
		}
		for _, id := range strip {
			if _, ok := m.symbols[id]; !ok {
				return nil, fmt.Errorf("slots[%s]: rouleau %d utilise le symbole inconnu %q", def.Key, i, id)
			}
		}
	}

	if len(def.Paylines) == 0 {
		return nil, fmt.Errorf("slots[%s]: aucune ligne de paiement", def.Key)
	}
	for j, line := range def.Paylines {
		if len(line) != len(def.Reels) {
			return nil, fmt.Errorf("slots[%s]: la ligne %d couvre %d rouleaux au lieu de %d",
				def.Key, j, len(line), len(def.Reels))
		}
		for _, row := range line {
			if row < 0 || row >= def.Rows {
				return nil, fmt.Errorf("slots[%s]: la ligne %d sort de la grille (ligne %d)", def.Key, j, row)
			}
		}
	}

	for id, table := range def.Paytable {
		if _, ok := m.symbols[id]; !ok {
			return nil, fmt.Errorf("slots[%s]: table de gains pour le symbole inconnu %q", def.Key, id)
		}
		for count := range table {
			if count < 2 || count > len(def.Reels) {
				return nil, fmt.Errorf("slots[%s]: gain declare pour %d symboles alignes, hors des bornes", def.Key, count)
			}
		}
	}
	if len(def.ScatterPays) > 0 && m.scatter == "" {
		return nil, fmt.Errorf("slots[%s]: gains de scatter declares sans symbole scatter", def.Key)
	}
	return m, nil
}

func MustCompile(def Definition) *Machine {
	m, err := Compile(def)
	if err != nil {
		panic(err)
	}
	return m
}

func (m *Machine) Key() string         { return m.def.Key }
func (m *Machine) Name() string        { return m.def.Name }
func (m *Machine) Description() string { return m.def.Description }
func (m *Machine) MinBet() int64       { return m.def.MinBetCredits }
func (m *Machine) MaxBet() int64       { return m.def.MaxBetCredits }

func (m *Machine) Config() map[string]any {
	return map[string]any{
		"type":            "slots",
		"rows":            m.def.Rows,
		"reel_count":      len(m.def.Reels),
		"reels":           m.def.Reels,
		"paylines":        m.def.Paylines,
		"symbols":         m.def.Symbols,
		"paytable":        m.def.Paytable,
		"scatter_pays":    m.def.ScatterPays,
		"wild_multiplier": m.def.WildMultiplier,
		"payout_unit":     "pour-mille de la mise totale",
		"theme":           m.def.Theme,
		"rtp":             m.def.TheoreticalRTP,
		"volatility":      m.def.Volatility,
		"bet_options": []map[string]any{
			{"id": "spin", "label": "Lancer", "description": "Un tour sur toutes les lignes"},
		},
	}
}

func (m *Machine) ValidateBet(b games.Bet) error {
	if b.Option != "" && b.Option != "spin" {
		return fmt.Errorf("option de mise inconnue pour une machine a sous: %q", b.Option)
	}
	return nil
}

type LineWin struct {
	Line      int     `json:"line"`
	Symbol    string  `json:"symbol"`
	Count     int     `json:"count"`
	WithWild  bool    `json:"with_wild"`
	Payout    int64   `json:"payout"`
	Positions [][]int `json:"positions"`
}

func (m *Machine) Play(r rng.Source, b games.Bet) (games.Outcome, error) {
	reelCount := len(m.def.Reels)
	stops := make([]int, reelCount)

	grid := make([][]string, m.def.Rows)
	for row := range grid {
		grid[row] = make([]string, reelCount)
	}
	for reel, strip := range m.def.Reels {
		stop := r.Intn(len(strip))
		stops[reel] = stop
		for row := 0; row < m.def.Rows; row++ {
			grid[row][reel] = strip[(stop+row)%len(strip)]
		}
	}

	var total int64
	lineWins := make([]LineWin, 0, len(m.def.Paylines))
	for j, line := range m.def.Paylines {
		win, ok := m.evaluateLine(grid, j, line, b.Amount)
		if !ok {
			continue
		}
		total += win.Payout
		lineWins = append(lineWins, win)
	}

	scatterCount := 0
	var scatterWin int64
	if m.scatter != "" {
		for _, row := range grid {
			for _, id := range row {
				if id == m.scatter {
					scatterCount++
				}
			}
		}
		if perMille, ok := m.def.ScatterPays[scatterCount]; ok && perMille > 0 {
			scatterWin = b.Amount * perMille / 1000
			total += scatterWin
		}
	}

	return games.Outcome{
		Payout: total,
		Label:  m.label(lineWins, scatterCount, total),
		Detail: map[string]any{
			"grid":          grid,
			"stops":         stops,
			"line_wins":     lineWins,
			"scatter_count": scatterCount,
			"scatter_win":   scatterWin,
			"total_payout":  total,
		},
	}, nil
}

func (m *Machine) evaluateLine(grid [][]string, index int, line []int, bet int64) (LineWin, bool) {
	symbols := make([]string, len(line))
	for reel, row := range line {
		symbols[reel] = grid[row][reel]
	}

	best := LineWin{Line: index}
	found := false

	for candidate, info := range m.symbols {
		if info.Scatter {
			continue
		}
		count := 0
		usedWild := false
		matchedReal := false
	scan:
		for _, id := range symbols {
			switch {
			case id == candidate:
				matchedReal = true
			case m.wilds[id]:
				usedWild = true
			default:
				break scan
			}
			count++
		}
		if count == 0 {
			continue
		}

		if !matchedReal && !info.Wild {
			continue
		}
		perMille, ok := m.def.Paytable[candidate][count]
		if !ok || perMille == 0 {
			continue
		}
		if usedWild && m.def.WildMultiplier > 1 {
			perMille *= m.def.WildMultiplier
		}
		payout := bet * perMille / 1000
		if payout <= best.Payout {
			continue
		}
		positions := make([][]int, 0, count)
		for reel := 0; reel < count; reel++ {
			positions = append(positions, []int{line[reel], reel})
		}
		best = LineWin{
			Line:      index,
			Symbol:    candidate,
			Count:     count,
			WithWild:  usedWild,
			Payout:    payout,
			Positions: positions,
		}
		found = true
	}
	return best, found && best.Payout > 0
}

func (m *Machine) label(wins []LineWin, scatters int, total int64) string {
	if total == 0 {
		return "Pas de combinaison"
	}
	if len(wins) > 0 {
		top := wins[0]
		for _, w := range wins[1:] {
			if w.Payout > top.Payout {
				top = w
			}
		}
		name := top.Symbol
		if s, ok := m.symbols[top.Symbol]; ok && s.Name != "" {
			name = s.Name
		}
		if len(wins) > 1 {
			return fmt.Sprintf("%d x %s (+%d ligne(s))", top.Count, name, len(wins)-1)
		}
		return fmt.Sprintf("%d x %s", top.Count, name)
	}
	return fmt.Sprintf("%d scatters", scatters)
}
