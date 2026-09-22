package roulette

import (
	"math"
	"testing"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/rng"
)

type forced struct{ n int }

func (f forced) Intn(int) int { return f.n }

func play(t *testing.T, result int, option string, amount int64) games.Outcome {
	t.Helper()
	out, err := New().Play(forced{n: result}, games.Bet{Amount: amount, Option: option})
	if err != nil {
		t.Fatalf("Play(%s): %v", option, err)
	}
	return out
}

func TestPayouts(t *testing.T) {
	cases := []struct {
		option string
		result int
		want   int64
	}{
		{"red", 1, 200},
		{"red", 2, 0},
		{"black", 2, 200},
		{"even", 2, 200},
		{"odd", 2, 0},
		{"low", 18, 200},
		{"high", 18, 0},
		{"high", 19, 200},
		{"dozen1", 12, 300},
		{"dozen2", 13, 300},
		{"dozen3", 24, 0},
		{"column1", 34, 300},
		{"column2", 35, 300},
		{"column3", 36, 300},
		{"number:17", 17, 3600},
		{"number:17", 18, 0},
		{"number:0", 0, 3600},
	}
	for _, tc := range cases {
		t.Run(tc.option, func(t *testing.T) {
			if got := play(t, tc.result, tc.option, 100).Payout; got != tc.want {
				t.Fatalf("%s sur %d: gain %d, attendu %d", tc.option, tc.result, got, tc.want)
			}
		})
	}
}

func TestZeroLosesOutsideBets(t *testing.T) {
	for _, option := range []string{"red", "black", "even", "odd", "low", "high",
		"dozen1", "dozen2", "dozen3", "column1", "column2", "column3", "number:5"} {
		if got := play(t, 0, option, 100).Payout; got != 0 {
			t.Fatalf("%s gagne sur le zero (gain %d)", option, got)
		}
	}
	if got := play(t, 0, "number:0", 100).Payout; got != 3600 {
		t.Fatalf("le plein sur le zero devrait payer 3600, obtenu %d", got)
	}
}

func TestRejectsInvalidOptions(t *testing.T) {
	g := New()
	for _, option := range []string{"", "vert", "number", "number:37", "number:-1",
		"number:abc", "red:3", "dozen4"} {
		if err := g.ValidateBet(games.Bet{Amount: 10, Option: option}); err == nil {
			t.Fatalf("option invalide acceptee: %q", option)
		}
	}
}

func TestRTP(t *testing.T) {
	if testing.Short() {
		t.Skip("simulation longue, ignoree en mode -short")
	}
	const (
		spins = 300000
		bet   = int64(100)
	)
	want := 36.0 / 37.0

	g := New()
	for _, option := range []string{"red", "even", "dozen1", "column2", "number:17"} {
		t.Run(option, func(t *testing.T) {
			r := rng.NewSeeded(20250922)
			var total int64
			for i := 0; i < spins; i++ {
				out, err := g.Play(r, games.Bet{Amount: bet, Option: option})
				if err != nil {
					t.Fatalf("Play: %v", err)
				}
				total += out.Payout
			}
			rtp := float64(total) / float64(int64(spins)*bet)
			t.Logf("%s : RTP mesure %.2f %% (theorique %.2f %%)", option, rtp*100, want*100)

			if math.Abs(rtp-want) > 0.05 {
				t.Fatalf("%s : RTP mesure %.4f, theorique %.4f", option, rtp, want)
			}
		})
	}
}

func TestColorsMatchTheWheel(t *testing.T) {
	var red, black, green int
	for n := 0; n < pockets; n++ {
		switch color(n) {
		case "red":
			red++
		case "black":
			black++
		case "green":
			green++
		}
	}
	if red != 18 || black != 18 || green != 1 {
		t.Fatalf("repartition %d rouges, %d noirs, %d verts : attendu 18/18/1", red, black, green)
	}
}
