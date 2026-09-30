package roulette

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/rng"
)

const pockets = 37

var redNumbers = map[int]bool{
	1: true, 3: true, 5: true, 7: true, 9: true, 12: true, 14: true, 16: true,
	18: true, 19: true, 21: true, 23: true, 25: true, 27: true, 30: true,
	32: true, 34: true, 36: true,
}

type BetKind struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Multiplier  int64  `json:"multiplier"`

	NeedsNumber bool `json:"needs_number"`
}

var betKinds = []BetKind{
	{ID: "red", Label: "Rouge", Description: "Le numero sorti est rouge", Multiplier: 2},
	{ID: "black", Label: "Noir", Description: "Le numero sorti est noir", Multiplier: 2},
	{ID: "even", Label: "Pair", Description: "Numero pair, le zero perd", Multiplier: 2},
	{ID: "odd", Label: "Impair", Description: "Numero impair", Multiplier: 2},
	{ID: "low", Label: "Manque (1-18)", Description: "Numero de 1 a 18", Multiplier: 2},
	{ID: "high", Label: "Passe (19-36)", Description: "Numero de 19 a 36", Multiplier: 2},
	{ID: "dozen1", Label: "1re douzaine", Description: "Numero de 1 a 12", Multiplier: 3},
	{ID: "dozen2", Label: "2e douzaine", Description: "Numero de 13 a 24", Multiplier: 3},
	{ID: "dozen3", Label: "3e douzaine", Description: "Numero de 25 a 36", Multiplier: 3},
	{ID: "column1", Label: "1re colonne", Description: "1, 4, 7... 34", Multiplier: 3},
	{ID: "column2", Label: "2e colonne", Description: "2, 5, 8... 35", Multiplier: 3},
	{ID: "column3", Label: "3e colonne", Description: "3, 6, 9... 36", Multiplier: 3},
	{ID: "number", Label: "Plein", Description: "Un numero precis, de 0 a 36", Multiplier: 36, NeedsNumber: true},
}

var kindByID = func() map[string]BetKind {
	m := make(map[string]BetKind, len(betKinds))
	for _, k := range betKinds {
		m[k.ID] = k
	}
	return m
}()

func BetKinds() []BetKind { return slices.Clone(betKinds) }

func Color(n int) string { return color(n) }

type Game struct{}

func New() *Game { return &Game{} }

func (g *Game) Key() string  { return "roulette" }
func (g *Game) Name() string { return "Roulette européenne" }
func (g *Game) Description() string {
	return "Roue à 37 cases, un seul zéro. Misez sur une couleur, une parité, une douzaine, une colonne ou un numéro plein."
}

func (g *Game) Config() map[string]any {
	colors := make(map[string]string, pockets)
	for n := 0; n < pockets; n++ {
		colors[strconv.Itoa(n)] = color(n)
	}
	return map[string]any{
		"type":        "roulette",
		"pockets":     pockets,
		"bet_options": betKinds,
		"colors":      colors,
		"rtp":         36.0 / 37.0,
		"theme": map[string]any{
			"palette":   map[string]string{"red": "#c0392b", "black": "#1c1c1c", "green": "#0f7b4f"},
			"animation": "wheel-spin",
		},
		"help": "Format de l'option : un identifiant de bet_options, et pour un plein 'number:<0-36>'.",
	}
}

func parseOption(option string) (BetKind, int, error) {
	option = strings.ToLower(strings.TrimSpace(option))
	if option == "" {
		return BetKind{}, 0, fmt.Errorf("aucun type de mise fourni")
	}

	id, arg, hasArg := strings.Cut(option, ":")
	kind, ok := kindByID[id]
	if !ok {
		return BetKind{}, 0, fmt.Errorf("type de mise inconnu: %q", id)
	}

	if !kind.NeedsNumber {
		if hasArg {
			return BetKind{}, 0, fmt.Errorf("le type de mise %q ne prend pas de numero", id)
		}
		return kind, -1, nil
	}

	if !hasArg {
		return BetKind{}, 0, fmt.Errorf("une mise en plein doit preciser un numero, par exemple number:17")
	}
	n, err := strconv.Atoi(arg)
	if err != nil {
		return BetKind{}, 0, fmt.Errorf("numero invalide: %q", arg)
	}
	if n < 0 || n >= pockets {
		return BetKind{}, 0, fmt.Errorf("le numero doit etre compris entre 0 et %d", pockets-1)
	}
	return kind, n, nil
}

func (g *Game) ValidateBet(b games.Bet) error {
	_, _, err := parseOption(b.Option)
	return err
}

func (g *Game) Play(r rng.Source, b games.Bet) (games.Outcome, error) {
	kind, number, err := parseOption(b.Option)
	if err != nil {
		return games.Outcome{}, err
	}

	result := r.Intn(pockets)
	won := wins(kind.ID, number, result)

	var payout int64
	if won {
		payout = b.Amount * kind.Multiplier
	}

	label := fmt.Sprintf("%d %s", result, colorFR(result))
	if won {
		label += " - gagne"
	} else {
		label += " - perdu"
	}

	return games.Outcome{
		Payout: payout,
		Label:  label,
		Detail: map[string]any{
			"result":     result,
			"color":      color(result),
			"bet_kind":   kind.ID,
			"bet_number": number,
			"multiplier": kind.Multiplier,
			"won":        won,
		},
	}, nil
}

func wins(kindID string, number, result int) bool {
	if result == 0 {
		return kindID == "number" && number == 0
	}
	switch kindID {
	case "red":
		return redNumbers[result]
	case "black":
		return !redNumbers[result]
	case "even":
		return result%2 == 0
	case "odd":
		return result%2 == 1
	case "low":
		return result >= 1 && result <= 18
	case "high":
		return result >= 19 && result <= 36
	case "dozen1":
		return result >= 1 && result <= 12
	case "dozen2":
		return result >= 13 && result <= 24
	case "dozen3":
		return result >= 25 && result <= 36
	case "column1":
		return result%3 == 1
	case "column2":
		return result%3 == 2
	case "column3":
		return result%3 == 0
	case "number":
		return result == number
	default:
		return false
	}
}

func color(n int) string {
	switch {
	case n == 0:
		return "green"
	case redNumbers[n]:
		return "red"
	default:
		return "black"
	}
}

func colorFR(n int) string {
	switch color(n) {
	case "green":
		return "vert"
	case "red":
		return "rouge"
	default:
		return "noir"
	}
}
