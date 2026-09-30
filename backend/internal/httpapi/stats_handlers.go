package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/casino-rt/backend/internal/games"
)

func (s *Server) handleHistory(c *gin.Context) {
	user := currentUser(c)
	limit := intQuery(c, "limit", 25, 1, 100)
	offset := intQuery(c, "offset", 0, 0, 100000)

	game := c.Query("game")
	if game != "" {
		if _, err := games.Get(game); err != nil {
			fail(c, http.StatusBadRequest, "unknown_game", "jeu inconnu")
			return
		}
	}

	rounds, total, err := s.loadHistory(c.Request.Context(), user.ID, game, limit, offset)
	if err != nil {
		slog.Error("historique", "err", err)
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

func (s *Server) handleStats(c *gin.Context) {
	stats, err := s.loadStats(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		slog.Error("statistiques", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (s *Server) handleLeaderboard(c *gin.Context) {
	limit := intQuery(c, "limit", 10, 1, 100)
	metric := c.DefaultQuery("metric", "balance")

	board, err := s.loadLeaderboard(c.Request.Context(), metric, limit)
	switch {
	case errors.Is(err, errInvalidMetric):
		fail(c, http.StatusBadRequest, "invalid_metric", err.Error())
		return
	case err != nil:
		slog.Error("classement", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	c.JSON(http.StatusOK, gin.H{"metric": metric, "leaderboard": board})
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
