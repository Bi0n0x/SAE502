package slots

func BuiltIn() []Definition {
	return []Definition{neon777(), cyberJungle()}
}

func repeatStrip(strip []string, reels int) [][]string {
	out := make([][]string, reels)
	for i := range out {
		out[i] = strip
	}
	return out
}

func neon777() Definition {
	strip := []string{
		"cherry", "lemon", "bell", "cherry", "bar",
		"lemon", "seven", "cherry", "bell", "lemon",
		"wild", "bar", "cherry", "bell", "lemon",
		"seven", "cherry", "bar", "bell", "lemon",
	}

	return Definition{
		Key:         "neon777",
		Name:        "Neon 777",
		Description: "Machine 5 rouleaux, 3 lignes, 5 lignes de paiement. Le wild double le gain de la ligne où il apparaît.",

		Rows:  3,
		Reels: repeatStrip(strip, 5),
		Paylines: [][]int{
			{1, 1, 1, 1, 1},
			{0, 0, 0, 0, 0},
			{2, 2, 2, 2, 2},
			{0, 1, 2, 1, 0},
			{2, 1, 0, 1, 2},
		},

		Symbols: []Symbol{
			{ID: "cherry", Name: "Cerise", Emoji: "\U0001F352", Color: "#e2445c"},
			{ID: "lemon", Name: "Citron", Emoji: "\U0001F34B", Color: "#f2c94c"},
			{ID: "bell", Name: "Cloche", Emoji: "\U0001F514", Color: "#f2994a"},
			{ID: "bar", Name: "Bar", Emoji: "\U0001F4B3", Color: "#56ccf2"},
			{ID: "seven", Name: "Sept", Emoji: "7️⃣", Color: "#eb5757"},
			{ID: "wild", Name: "Wild Neon", Emoji: "⭐", Color: "#bb6bd9", Wild: true},
		},

		Paytable: map[string]map[int]int64{
			"cherry": {3: 250, 4: 800, 5: 2600},
			"lemon":  {3: 400, 4: 1000, 5: 4000},
			"bell":   {3: 800, 4: 2000, 5: 8000},
			"bar":    {3: 1600, 4: 5000, 5: 18000},
			"seven":  {3: 5000, 4: 20000, 5: 100000},
			"wild":   {3: 8000, 4: 40000, 5: 250000},
		},
		WildMultiplier: 2,

		Theme: map[string]any{
			"palette":    map[string]string{"bg": "#120d2b", "accent": "#bb6bd9", "win": "#f2c94c"},
			"animation":  "neon-pulse",
			"reel_speed": 90,
			"sound":      "arcade",
		},

		MinBetCredits:  1,
		MaxBetCredits:  200,
		TheoreticalRTP: 0.9148,
		Volatility:     "moyenne",
	}
}

func cyberJungle() Definition {
	strip := []string{
		"leaf", "monkey", "tiger", "leaf", "totem", "monkey",
		"leaf", "idol", "tiger", "monkey", "leaf", "wild",
		"monkey", "tiger", "leaf", "totem", "monkey", "leaf",
		"scatter", "tiger", "monkey", "leaf", "totem", "idol",
	}

	return Definition{
		Key:         "cyberjungle",
		Name:        "Cyber Jungle",
		Description: "5 rouleaux, 9 lignes. Le wild triple la ligne, et 3 scatters ou plus paient sans être alignés.",

		Rows:  3,
		Reels: repeatStrip(strip, 5),
		Paylines: [][]int{
			{1, 1, 1, 1, 1},
			{0, 0, 0, 0, 0},
			{2, 2, 2, 2, 2},
			{0, 1, 2, 1, 0},
			{2, 1, 0, 1, 2},
			{0, 0, 1, 0, 0},
			{2, 2, 1, 2, 2},
			{1, 0, 0, 0, 1},
			{1, 2, 2, 2, 1},
		},

		Symbols: []Symbol{
			{ID: "leaf", Name: "Feuille", Emoji: "\U0001F343", Color: "#6fcf97"},
			{ID: "monkey", Name: "Singe", Emoji: "\U0001F412", Color: "#bb8a52"},
			{ID: "tiger", Name: "Tigre", Emoji: "\U0001F42F", Color: "#f2994a"},
			{ID: "totem", Name: "Totem", Emoji: "\U0001F5FF", Color: "#56ccf2"},
			{ID: "idol", Name: "Idole d'or", Emoji: "\U0001F451", Color: "#f2c94c"},
			{ID: "wild", Name: "Wild Cyber", Emoji: "⚡", Color: "#9b51e0", Wild: true},
			{ID: "scatter", Name: "Scatter", Emoji: "\U0001F4A0", Color: "#2f80ed", Scatter: true},
		},

		Paytable: map[string]map[int]int64{
			"leaf":   {3: 120, 4: 400, 5: 1250},
			"monkey": {3: 200, 4: 650, 5: 2000},
			"tiger":  {3: 400, 4: 1250, 5: 4000},
			"totem":  {3: 1000, 4: 3300, 5: 10000},
			"idol":   {3: 3300, 4: 12500, 5: 50000},
			"wild":   {3: 8000, 4: 40000, 5: 250000},
		},
		ScatterPays:    map[int]int64{3: 3000, 4: 15000, 5: 75000},
		WildMultiplier: 3,

		Theme: map[string]any{
			"palette":    map[string]string{"bg": "#06231c", "accent": "#6fcf97", "win": "#2f80ed"},
			"animation":  "vine-grow",
			"reel_speed": 75,
			"sound":      "jungle",
		},

		MinBetCredits:  1,
		MaxBetCredits:  300,
		TheoreticalRTP: 0.9292,
		Volatility:     "faible à moyenne",
	}
}
