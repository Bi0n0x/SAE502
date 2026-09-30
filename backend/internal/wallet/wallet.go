package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/models"
	"github.com/casino-rt/backend/internal/rng"
)

var (
	ErrInsufficientFunds = errors.New("solde insuffisant")
	ErrInvalidBet        = errors.New("mise invalide")
	ErrUserNotFound      = errors.New("joueur introuvable")
)

type Service struct {
	db     *gorm.DB
	rand   rng.Source
	minBet int64
	maxBet int64
}

func New(db *gorm.DB, r rng.Source, minBet, maxBet int64) *Service {
	return &Service{db: db, rand: r, minBet: minBet, maxBet: maxBet}
}

type PlayRequest struct {
	UserID uuid.UUID
	Amount int64
	Option string
	Params map[string]any

	IdempotencyKey *string
}

type PlayResult struct {
	Round      models.Round
	Detail     map[string]any
	NewBalance int64
	Replayed   bool
}

func (s *Service) PlayRound(ctx context.Context, g games.Game, req PlayRequest) (*PlayResult, error) {
	if err := s.checkAmount(g, req.Amount); err != nil {
		return nil, err
	}
	bet := games.Bet{Amount: req.Amount, Option: req.Option, Params: req.Params}
	if err := g.ValidateBet(bet); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidBet, err)
	}

	if req.IdempotencyKey != nil {
		if res, ok, err := s.findByIdempotencyKey(ctx, req.UserID, *req.IdempotencyKey); err != nil {
			return nil, err
		} else if ok {
			return res, nil
		}
	}

	var result PlayResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user models.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", req.UserID).
			Take(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}

		if user.Balance < req.Amount {
			return fmt.Errorf("%w: solde %d, mise %d", ErrInsufficientFunds, user.Balance, req.Amount)
		}

		outcome, err := g.Play(s.rand, bet)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidBet, err)
		}
		if outcome.Payout < 0 {
			return fmt.Errorf("jeu %s: gain negatif (%d), resultat rejete", g.Key(), outcome.Payout)
		}

		newBalance := user.Balance - req.Amount + outcome.Payout

		upd := tx.Model(&models.User{}).
			Where("id = ? AND balance >= ?", req.UserID, req.Amount).
			Updates(map[string]any{
				"balance":    gorm.Expr("balance - ? + ?", req.Amount, outcome.Payout),
				"updated_at": time.Now(),
			})
		if upd.Error != nil {
			return upd.Error
		}
		if upd.RowsAffected != 1 {
			return fmt.Errorf("%w: mise a jour du solde refusee", ErrInsufficientFunds)
		}

		detailJSON, err := json.Marshal(outcome.Detail)
		if err != nil {
			return fmt.Errorf("serialisation du resultat: %w", err)
		}

		round := models.Round{
			UserID:         req.UserID,
			Game:           g.Key(),
			BetAmount:      req.Amount,
			BetOption:      req.Option,
			Payout:         outcome.Payout,
			Net:            outcome.Payout - req.Amount,
			BalanceAfter:   newBalance,
			Label:          outcome.Label,
			Detail:         string(detailJSON),
			IdempotencyKey: req.IdempotencyKey,
		}
		if err := tx.Create(&round).Error; err != nil {
			return err
		}

		result = PlayResult{Round: round, Detail: outcome.Detail, NewBalance: newBalance}
		return nil
	})

	if err != nil && req.IdempotencyKey != nil && isDuplicateKey(err) {
		if res, ok, lookupErr := s.findByIdempotencyKey(ctx, req.UserID, *req.IdempotencyKey); lookupErr == nil && ok {
			return res, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Bounds returns the bet range accepted for g: the game's own limits, capped
// by the global ones.
func (s *Service) Bounds(g games.Game) (min, max int64) {
	min, max = s.minBet, s.maxBet
	if v, ok := g.(games.MinBetter); ok && v.MinBet() > 0 {
		min = v.MinBet()
	}
	if v, ok := g.(games.MaxBetter); ok && v.MaxBet() > 0 && v.MaxBet() < max {
		max = v.MaxBet()
	}
	return min, max
}

func (s *Service) checkAmount(g games.Game, amount int64) error {
	min, max := s.Bounds(g)
	if amount <= 0 {
		return fmt.Errorf("%w: la mise doit etre strictement positive", ErrInvalidBet)
	}
	if amount < min {
		return fmt.Errorf("%w: mise minimale de %d credits", ErrInvalidBet, min)
	}
	if amount > max {
		return fmt.Errorf("%w: mise maximale de %d credits", ErrInvalidBet, max)
	}
	return nil
}

func (s *Service) findByIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*PlayResult, bool, error) {
	var round models.Round
	err := s.db.WithContext(ctx).
		Where("idempotency_key = ? AND user_id = ?", key, userID).
		Take(&round).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	detail := map[string]any{}
	_ = json.Unmarshal([]byte(round.Detail), &detail)
	return &PlayResult{
		Round:      round,
		Detail:     detail,
		NewBalance: round.BalanceAfter,
		Replayed:   true,
	}, true, nil
}

func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}

func (s *Service) Balance(ctx context.Context, userID uuid.UUID) (int64, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrUserNotFound
		}
		return 0, err
	}
	return user.Balance, nil
}
