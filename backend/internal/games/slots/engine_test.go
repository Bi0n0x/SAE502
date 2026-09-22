package slots

import (
	"testing"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/rng"
)

type fixedRNG struct {
	stops []int
	i     int
}

func (f *fixedRNG) Intn(int) int {
	v := f.stops[f.i%len(f.stops)]
	f.i++
	return v
}

func testMachine(t *testing.T) *Machine {
	t.Helper()
	strip := []string{"a", "b", "wild", "c"}
	m, err := Compile(Definition{
		Key:      "test",
		Name:     "Test",
		Rows:     1,
		Reels:    [][]string{strip, strip, strip},
		Paylines: [][]int{{0, 0, 0}},
		Symbols: []Symbol{
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B"},
			{ID: "c", Name: "C"},
			{ID: "wild", Name: "Wild", Wild: true},
		},
		Paytable: map[string]map[int]int64{
			"a":    {2: 500, 3: 2000},
			"b":    {3: 1000},
			"wild": {3: 10000},
		},
		WildMultiplier: 2,
		MinBetCredits:  1,
		MaxBetCredits:  100,
	})
	if err != nil {
		t.Fatalf("compilation: %v", err)
	}
	return m
}

func spin(t *testing.T, m *Machine, stops []int, bet int64) games.Outcome {
	t.Helper()
	out, err := m.Play(&fixedRNG{stops: stops}, games.Bet{Amount: bet, Option: "spin"})
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	return out
}

func TestThreeOfAKindPays(t *testing.T) {
	m := testMachine(t)

	out := spin(t, m, []int{0, 0, 0}, 100)

	if out.Payout != 200 {
		t.Fatalf("gain %d, attendu 200", out.Payout)
	}
}

func TestWildSubstitutesAndMultiplies(t *testing.T) {
	m := testMachine(t)

	out := spin(t, m, []int{0, 0, 2}, 100)
	if out.Payout != 400 {
		t.Fatalf("gain %d, attendu 400 (gain double par le wild)", out.Payout)
	}
	wins := out.Detail["line_wins"].([]LineWin)
	if len(wins) != 1 || !wins[0].WithWild {
		t.Fatalf("la ligne gagnante devrait etre marquee comme contenant un wild: %+v", wins)
	}
}

func TestAllWildLineUsesWildPaytable(t *testing.T) {
	m := testMachine(t)
	out := spin(t, m, []int{2, 2, 2}, 100)

	if out.Payout != 1000 {
		t.Fatalf("gain %d, attendu 1000", out.Payout)
	}
	wins := out.Detail["line_wins"].([]LineWin)
	if len(wins) != 1 || wins[0].Symbol != "wild" || wins[0].WithWild {
		t.Fatalf("la ligne devrait payer au titre du wild, sans bonus de substitution: %+v", wins)
	}
}

func TestPicksBestCombination(t *testing.T) {
	m := testMachine(t)

	out := spin(t, m, []int{2, 0, 1}, 100)
	if out.Payout != 100 {
		t.Fatalf("gain %d, attendu 100", out.Payout)
	}
}

func TestNoCombinationPaysNothing(t *testing.T) {
	m := testMachine(t)

	out := spin(t, m, []int{1, 3, 0}, 100)
	if out.Payout != 0 {
		t.Fatalf("gain %d, attendu 0", out.Payout)
	}
	if out.Label != "Pas de combinaison" {
		t.Fatalf("libelle %q inattendu", out.Label)
	}
}

func TestCompileRejectsBadDefinitions(t *testing.T) {
	cases := []struct {
		name string
		def  Definition
	}{
		{"symbole inconnu dans un rouleau", Definition{
			Key: "x", Rows: 1,
			Reels:    [][]string{{"a"}, {"a"}, {"zzz"}},
			Paylines: [][]int{{0, 0, 0}},
			Symbols:  []Symbol{{ID: "a"}},
		}},
		{"ligne de paiement trop courte", Definition{
			Key: "x", Rows: 1,
			Reels:    [][]string{{"a"}, {"a"}, {"a"}},
			Paylines: [][]int{{0, 0}},
			Symbols:  []Symbol{{ID: "a"}},
		}},
		{"ligne hors grille", Definition{
			Key: "x", Rows: 1,
			Reels:    [][]string{{"a"}, {"a"}, {"a"}},
			Paylines: [][]int{{0, 0, 5}},
			Symbols:  []Symbol{{ID: "a"}},
		}},
		{"gains de scatter sans scatter", Definition{
			Key: "x", Rows: 1,
			Reels:       [][]string{{"a"}, {"a"}, {"a"}},
			Paylines:    [][]int{{0, 0, 0}},
			Symbols:     []Symbol{{ID: "a"}},
			ScatterPays: map[int]int64{3: 100},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compile(tc.def); err == nil {
				t.Fatal("definition invalide acceptee")
			}
		})
	}
}

func TestBuiltInMachinesCompile(t *testing.T) {
	for _, def := range BuiltIn() {
		if _, err := Compile(def); err != nil {
			t.Fatalf("machine %s invalide: %v", def.Key, err)
		}
	}
}

func TestRTP(t *testing.T) {
	if testing.Short() {
		t.Skip("simulation longue, ignoree en mode -short")
	}
	const (
		spins     = 2000000
		bet       = int64(1000)
		tolerance = 0.04
	)

	for _, def := range BuiltIn() {
		def := def
		t.Run(def.Key, func(t *testing.T) {
			m := MustCompile(def)
			r := rng.NewSeeded(20250922)

			var total int64
			hits := 0
			for i := 0; i < spins; i++ {
				out, err := m.Play(r, games.Bet{Amount: bet, Option: "spin"})
				if err != nil {
					t.Fatalf("Play: %v", err)
				}
				total += out.Payout
				if out.Payout > 0 {
					hits++
				}
			}

			rtp := float64(total) / float64(int64(spins)*bet)
			hitRate := float64(hits) / float64(spins)
			t.Logf("%s : RTP mesure %.2f %% (annonce %.2f %%), tours gagnants %.1f %%",
				def.Key, rtp*100, def.TheoreticalRTP*100, hitRate*100)

			if rtp > 1.0 {
				t.Fatalf("RTP de %.2f %% : la machine rend plus qu'elle ne prend", rtp*100)
			}
			if diff := rtp - def.TheoreticalRTP; diff > tolerance || diff < -tolerance {
				t.Fatalf("RTP mesure %.4f, annonce %.4f : ecart trop grand, mettre a jour TheoreticalRTP",
					rtp, def.TheoreticalRTP)
			}
		})
	}
}
