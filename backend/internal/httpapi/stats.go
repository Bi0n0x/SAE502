package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/casino-rt/backend/internal/models"
)

var errInvalidMetric = errors.New("metric doit valoir balance, net, rounds ou biggest_win")

type gameStat struct {
	Game     string  `json:"game"`
	Rounds   int64   `json:"rounds"`
	Wagered  int64   `json:"wagered"`
	Won      int64   `json:"won"`
	Net      int64   `json:"net"`
	WinRate  float64 `json:"win_rate"`
	BestWin  int64   `json:"best_win"`
	WorstNet int64   `json:"worst_net"`
}

type statsTotal struct {
	Rounds  int64   `json:"rounds"`
	Wagered int64   `json:"wagered"`
	Won     int64   `json:"won"`
	Net     int64   `json:"net"`
	WinRate float64 `json:"win_rate"`

	ReturnRate float64 `json:"return_rate"`
}

type userStats struct {
	Balance int64      `json:"balance"`
	Total   statsTotal `json:"total"`
	PerGame []gameStat `json:"per_game"`
}

type leaderEntry struct {
	Rank       int    `json:"rank"`
	Username   string `json:"username"`
	Balance    int64  `json:"balance"`
	Rounds     int64  `json:"rounds"`
	Net        int64  `json:"net"`
	BiggestWin int64  `json:"biggest_win"`
}

func (s *Server) loadHistory(ctx context.Context, userID uuid.UUID, game string, limit, offset int) ([]models.Round, int64, error) {
	q := s.db.WithContext(ctx).
		Model(&models.Round{}).
		Where("user_id = ?", userID)
	if game != "" {
		q = q.Where("game = ?", game)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("comptage de l'historique: %w", err)
	}

	var rounds []models.Round
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rounds).Error; err != nil {
		return nil, 0, fmt.Errorf("lecture de l'historique: %w", err)
	}
	return rounds, total, nil
}

func (s *Server) loadStats(ctx context.Context, userID uuid.UUID) (userStats, error) {
	var rows []struct {
		Game     string
		Rounds   int64
		Wagered  int64
		Won      int64
		Net      int64
		Wins     int64
		BestWin  int64
		WorstNet int64
	}
	err := s.db.WithContext(ctx).
		Model(&models.Round{}).
		Select(`game,
		        COUNT(*)                       AS rounds,
		        COALESCE(SUM(bet_amount), 0)   AS wagered,
		        COALESCE(SUM(payout), 0)       AS won,
		        COALESCE(SUM(net), 0)          AS net,
		        COUNT(*) FILTER (WHERE net > 0) AS wins,
		        COALESCE(MAX(net), 0)          AS best_win,
		        COALESCE(MIN(net), 0)          AS worst_net`).
		Where("user_id = ?", userID).
		Group("game").
		Order("game").
		Scan(&rows).Error
	if err != nil {
		return userStats{}, fmt.Errorf("calcul des statistiques: %w", err)
	}

	out := userStats{PerGame: make([]gameStat, 0, len(rows))}
	var totalWins int64
	for _, r := range rows {
		stat := gameStat{
			Game:     r.Game,
			Rounds:   r.Rounds,
			Wagered:  r.Wagered,
			Won:      r.Won,
			Net:      r.Net,
			BestWin:  r.BestWin,
			WorstNet: r.WorstNet,
		}
		if r.Rounds > 0 {
			stat.WinRate = float64(r.Wins) / float64(r.Rounds)
		}
		out.PerGame = append(out.PerGame, stat)

		out.Total.Rounds += r.Rounds
		out.Total.Wagered += r.Wagered
		out.Total.Won += r.Won
		out.Total.Net += r.Net
		totalWins += r.Wins
	}
	if out.Total.Rounds > 0 {
		out.Total.WinRate = float64(totalWins) / float64(out.Total.Rounds)
	}
	if out.Total.Wagered > 0 {
		out.Total.ReturnRate = float64(out.Total.Won) / float64(out.Total.Wagered)
	}

	out.Balance, err = s.wallet.Balance(ctx, userID)
	if err != nil {
		return userStats{}, fmt.Errorf("lecture du solde: %w", err)
	}
	return out, nil
}

func (s *Server) loadLeaderboard(ctx context.Context, metric string, limit int) ([]leaderEntry, error) {
	orderBy, ok := map[string]string{
		"balance":     "u.balance DESC",
		"net":         "net DESC",
		"rounds":      "rounds DESC",
		"biggest_win": "biggest_win DESC",
	}[metric]
	if !ok {
		return nil, errInvalidMetric
	}

	var rows []struct {
		Username   string
		Balance    int64
		Rounds     int64
		Net        int64
		BiggestWin int64
	}
	err := s.db.WithContext(ctx).
		Table("users AS u").
		Select(`u.username                          AS username,
		        u.balance                           AS balance,
		        COALESCE(COUNT(r.id), 0)            AS rounds,
		        COALESCE(SUM(r.net), 0)             AS net,
		        COALESCE(MAX(r.net), 0)             AS biggest_win`).
		Joins("LEFT JOIN rounds AS r ON r.user_id = u.id").
		Group("u.id, u.username, u.balance").
		Order(orderBy).
		Order("u.username ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("calcul du classement: %w", err)
	}

	out := make([]leaderEntry, 0, len(rows))
	for i, r := range rows {
		out = append(out, leaderEntry{
			Rank:       i + 1,
			Username:   r.Username,
			Balance:    r.Balance,
			Rounds:     r.Rounds,
			Net:        r.Net,
			BiggestWin: r.BiggestWin,
		})
	}
	return out, nil
}
