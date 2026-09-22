package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/models"
)

func (s *Server) handleHistory(c *gin.Context) {
	user := currentUser(c)
	limit := intQuery(c, "limit", 25, 1, 100)
	offset := intQuery(c, "offset", 0, 0, 100000)

	q := s.db.WithContext(c.Request.Context()).
		Model(&models.Round{}).
		Where("user_id = ?", user.ID)

	if game := c.Query("game"); game != "" {
		if _, err := games.Get(game); err != nil {
			fail(c, http.StatusBadRequest, "unknown_game", "jeu inconnu")
			return
		}
		q = q.Where("game = ?", game)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		slog.Error("comptage de l'historique", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	var rounds []models.Round
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rounds).Error; err != nil {
		slog.Error("lecture de l'historique", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	out := make([]roundView, 0, len(rounds))
	for _, r := range rounds {
		out = append(out, roundResponse(r, nil))
	}
	c.JSON(http.StatusOK, gin.H{
		"rounds": out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

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

func (s *Server) handleStats(c *gin.Context) {
	user := currentUser(c)
	ctx := c.Request.Context()

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
		Where("user_id = ?", user.ID).
		Group("game").
		Order("game").
		Scan(&rows).Error
	if err != nil {
		slog.Error("calcul des statistiques", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	perGame := make([]gameStat, 0, len(rows))
	var totalRounds, totalWagered, totalWon, totalNet, totalWins int64
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
		perGame = append(perGame, stat)

		totalRounds += r.Rounds
		totalWagered += r.Wagered
		totalWon += r.Won
		totalNet += r.Net
		totalWins += r.Wins
	}

	balance, err := s.wallet.Balance(ctx, user.ID)
	if err != nil {
		slog.Error("lecture du solde", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	total := gin.H{
		"rounds":   totalRounds,
		"wagered":  totalWagered,
		"won":      totalWon,
		"net":      totalNet,
		"win_rate": 0.0,

		"return_rate": 0.0,
	}
	if totalRounds > 0 {
		total["win_rate"] = float64(totalWins) / float64(totalRounds)
	}
	if totalWagered > 0 {
		total["return_rate"] = float64(totalWon) / float64(totalWagered)
	}

	c.JSON(http.StatusOK, gin.H{
		"balance":  balance,
		"total":    total,
		"per_game": perGame,
	})
}

func (s *Server) handleLeaderboard(c *gin.Context) {
	limit := intQuery(c, "limit", 10, 1, 100)
	metric := c.DefaultQuery("metric", "balance")

	orderBy, ok := map[string]string{
		"balance":     "u.balance DESC",
		"net":         "net DESC",
		"rounds":      "rounds DESC",
		"biggest_win": "biggest_win DESC",
	}[metric]
	if !ok {
		fail(c, http.StatusBadRequest, "invalid_metric",
			"metric doit valoir balance, net, rounds ou biggest_win")
		return
	}

	var rows []struct {
		Username   string
		Balance    int64
		Rounds     int64
		Net        int64
		BiggestWin int64
	}

	err := s.db.WithContext(c.Request.Context()).
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
		slog.Error("calcul du classement", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	out := make([]gin.H, 0, len(rows))
	for i, r := range rows {
		out = append(out, gin.H{
			"rank":        i + 1,
			"username":    r.Username,
			"balance":     r.Balance,
			"rounds":      r.Rounds,
			"net":         r.Net,
			"biggest_win": r.BiggestWin,
		})
	}
	c.JSON(http.StatusOK, gin.H{"metric": metric, "leaderboard": out})
}

func intQuery(c *gin.Context, name string, def, min, max int) int {
	raw := c.Query(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
