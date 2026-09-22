package wallet_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/models"
	"github.com/casino-rt/backend/internal/rng"
	"github.com/casino-rt/backend/internal/wallet"
)

type fixedGame struct {
	key        string
	multiplier int64
}

func (g fixedGame) Key() string                 { return g.key }
func (g fixedGame) Name() string                { return g.key }
func (g fixedGame) Description() string         { return "jeu de test" }
func (g fixedGame) Config() map[string]any      { return map[string]any{"type": "test"} }
func (g fixedGame) ValidateBet(games.Bet) error { return nil }
func (g fixedGame) Play(_ rng.Source, b games.Bet) (games.Outcome, error) {
	return games.Outcome{
		Payout: b.Amount * g.multiplier,
		Label:  "test",
		Detail: map[string]any{"multiplier": g.multiplier},
	}, nil
}

var (
	alwaysLose = fixedGame{key: "test_lose", multiplier: 0}
	alwaysWin  = fixedGame{key: "test_win", multiplier: 2}
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL non defini : test de concurrence ignore")
	}
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("connexion a la base de test: %v", err)
	}
	if err := gdb.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migration: %v", err)
	}
	sqlDB, _ := gdb.DB()
	sqlDB.SetMaxOpenConns(30)
	return gdb
}

func newPlayer(t *testing.T, gdb *gorm.DB, balance int64) models.User {
	t.Helper()
	u := models.User{
		Username:     "test_" + uuid.NewString()[:12],
		PasswordHash: "x",
		Balance:      balance,
	}
	if err := gdb.Create(&u).Error; err != nil {
		t.Fatalf("creation du joueur de test: %v", err)
	}
	t.Cleanup(func() {
		gdb.Where("user_id = ?", u.ID).Delete(&models.Round{})
		gdb.Where("id = ?", u.ID).Delete(&models.User{})
	})
	return u
}

func balanceOf(t *testing.T, gdb *gorm.DB, id uuid.UUID) int64 {
	t.Helper()
	var u models.User
	if err := gdb.Where("id = ?", id).Take(&u).Error; err != nil {
		t.Fatalf("relecture du solde: %v", err)
	}
	return u.Balance
}

func TestConcurrentBetsNoLostUpdate(t *testing.T) {
	gdb := testDB(t)
	svc := wallet.New(gdb, rng.Crypto{}, 1, 1000)

	const (
		start   int64 = 10000
		bet     int64 = 10
		workers       = 50
	)
	user := newPlayer(t, gdb, start)

	var wg sync.WaitGroup
	var ok, failed atomic.Int64
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.PlayRound(context.Background(), alwaysLose, wallet.PlayRequest{
				UserID: user.ID,
				Amount: bet,
			})
			if err != nil {
				failed.Add(1)
				t.Errorf("mise refusee alors que le solde suffisait: %v", err)
				return
			}
			ok.Add(1)
		}()
	}
	wg.Wait()

	want := start - bet*workers
	if got := balanceOf(t, gdb, user.ID); got != want {
		t.Fatalf("solde final %d, attendu %d (%d mises acceptees, %d refusees) : "+
			"une mise a ete perdue ou appliquee deux fois", got, want, ok.Load(), failed.Load())
	}

	var rounds int64
	gdb.Model(&models.Round{}).Where("user_id = ?", user.ID).Count(&rounds)
	if rounds != workers {
		t.Fatalf("%d parties enregistrees, attendu %d", rounds, workers)
	}
}

func TestConcurrentBetsCannotOverdraw(t *testing.T) {
	gdb := testDB(t)
	svc := wallet.New(gdb, rng.Crypto{}, 1, 1000)

	const (
		start   int64 = 1000
		bet     int64 = 100
		workers       = 50
	)
	user := newPlayer(t, gdb, start)

	var wg sync.WaitGroup
	var accepted, rejected atomic.Int64
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.PlayRound(context.Background(), alwaysLose, wallet.PlayRequest{
				UserID: user.ID,
				Amount: bet,
			})
			switch {
			case err == nil:
				accepted.Add(1)
			case errors.Is(err, wallet.ErrInsufficientFunds):
				rejected.Add(1)
			default:
				t.Errorf("erreur inattendue: %v", err)
			}
		}()
	}
	wg.Wait()

	if accepted.Load() != start/bet {
		t.Fatalf("%d mises acceptees, attendu %d", accepted.Load(), start/bet)
	}
	if got := balanceOf(t, gdb, user.ID); got != 0 {
		t.Fatalf("solde final %d, attendu 0", got)
	}
	if rejected.Load() != workers-start/bet {
		t.Fatalf("%d refus, attendu %d", rejected.Load(), workers-start/bet)
	}
}

func TestConcurrentWinsAndLosses(t *testing.T) {
	gdb := testDB(t)
	svc := wallet.New(gdb, rng.Crypto{}, 1, 1000)

	const (
		start   int64 = 5000
		bet     int64 = 20
		workers       = 60
	)
	user := newPlayer(t, gdb, start)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		game := alwaysLose
		if i%2 == 0 {
			game = alwaysWin
		}
		go func(g fixedGame) {
			defer wg.Done()
			if _, err := svc.PlayRound(context.Background(), g, wallet.PlayRequest{
				UserID: user.ID,
				Amount: bet,
			}); err != nil {
				t.Errorf("mise refusee: %v", err)
			}
		}(game)
	}
	wg.Wait()

	var sumNet int64
	gdb.Model(&models.Round{}).
		Where("user_id = ?", user.ID).
		Select("COALESCE(SUM(net), 0)").
		Scan(&sumNet)

	want := start + sumNet
	if got := balanceOf(t, gdb, user.ID); got != want {
		t.Fatalf("solde %d mais historique %d : le journal et le solde ont diverge", got, want)
	}
}

func TestIdempotencyKeyPreventsDoubleBet(t *testing.T) {
	gdb := testDB(t)
	svc := wallet.New(gdb, rng.Crypto{}, 1, 1000)

	user := newPlayer(t, gdb, 1000)
	key := user.ID.String() + ":" + uuid.NewString()

	const workers = 12
	var wg sync.WaitGroup
	results := make([]*wallet.PlayResult, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			res, err := svc.PlayRound(context.Background(), alwaysLose, wallet.PlayRequest{
				UserID:         user.ID,
				Amount:         100,
				IdempotencyKey: &key,
			})
			if err != nil {
				t.Errorf("mise %d refusee: %v", i, err)
				return
			}
			results[i] = res
		}(i)
	}
	wg.Wait()

	if got := balanceOf(t, gdb, user.ID); got != 900 {
		t.Fatalf("solde %d, attendu 900 : la mise a ete prise en compte plusieurs fois", got)
	}
	var rounds int64
	gdb.Model(&models.Round{}).Where("user_id = ?", user.ID).Count(&rounds)
	if rounds != 1 {
		t.Fatalf("%d parties enregistrees, attendu 1", rounds)
	}
	for i, r := range results {
		if r == nil {
			continue
		}
		if r.Round.ID != results[0].Round.ID && results[0] != nil {
			t.Fatalf("la reponse %d ne designe pas la meme partie", i)
		}
	}
}

func TestLoadThirtyPlayers(t *testing.T) {
	gdb := testDB(t)
	svc := wallet.New(gdb, rng.Crypto{}, 1, 1000)

	const (
		players          = 30
		roundsEach       = 20
		start      int64 = 2000
		bet        int64 = 10
	)

	users := make([]models.User, players)
	for i := range users {
		users[i] = newPlayer(t, gdb, start)
	}

	var wg sync.WaitGroup
	wg.Add(players)
	for i := range users {
		go func(u models.User) {
			defer wg.Done()
			for r := 0; r < roundsEach; r++ {
				if _, err := svc.PlayRound(context.Background(), alwaysLose, wallet.PlayRequest{
					UserID: u.ID,
					Amount: bet,
				}); err != nil {
					t.Errorf("joueur %s, tour %d: %v", u.Username, r, err)
					return
				}
			}
		}(users[i])
	}
	wg.Wait()

	want := start - bet*roundsEach
	for _, u := range users {
		if got := balanceOf(t, gdb, u.ID); got != want {
			t.Fatalf("joueur %s: solde %d, attendu %d", u.Username, got, want)
		}
	}
}

func TestRejectsInvalidBets(t *testing.T) {
	gdb := testDB(t)
	svc := wallet.New(gdb, rng.Crypto{}, 1, 500)
	user := newPlayer(t, gdb, 100)

	cases := []struct {
		name   string
		amount int64
		want   error
	}{
		{"mise nulle", 0, wallet.ErrInvalidBet},
		{"mise negative", -50, wallet.ErrInvalidBet},
		{"au-dessus du plafond", 501, wallet.ErrInvalidBet},
		{"au-dessus du solde", 200, wallet.ErrInsufficientFunds},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.PlayRound(context.Background(), alwaysLose, wallet.PlayRequest{
				UserID: user.ID,
				Amount: tc.amount,
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("erreur %v, attendu %v", err, tc.want)
			}
		})
	}

	if got := balanceOf(t, gdb, user.ID); got != 100 {
		t.Fatalf("solde modifie (%d) alors que toutes les mises ont ete refusees", got)
	}
	var rounds int64
	gdb.Model(&models.Round{}).Where("user_id = ?", user.ID).Count(&rounds)
	if rounds != 0 {
		t.Fatalf("%d parties enregistrees alors qu'aucune mise n'a ete acceptee", rounds)
	}
}
