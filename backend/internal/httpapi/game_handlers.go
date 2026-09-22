package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/models"
	"github.com/casino-rt/backend/internal/wallet"
)

func (s *Server) handleListGames(c *gin.Context) {
	all := games.All()
	out := make([]map[string]any, 0, len(all))
	for _, g := range all {
		out = append(out, games.Describe(g))
	}
	c.JSON(http.StatusOK, gin.H{"games": out})
}

func (s *Server) handleGetGame(c *gin.Context) {
	g, err := games.Get(c.Param("key"))
	if err != nil {
		fail(c, http.StatusNotFound, "unknown_game", "jeu inconnu")
		return
	}
	c.JSON(http.StatusOK, gin.H{"game": games.Describe(g)})
}

type playRequest struct {
	Amount int64          `json:"amount"`
	Option string         `json:"option"`
	Params map[string]any `json:"params"`

	IdempotencyKey string `json:"idempotency_key"`
}

func (s *Server) handlePlay(c *gin.Context) {
	g, err := games.Get(c.Param("key"))
	if err != nil {
		fail(c, http.StatusNotFound, "unknown_game", "jeu inconnu")
		return
	}

	var in playRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "corps de requete invalide")
		return
	}

	var key *string
	if k := strings.TrimSpace(in.IdempotencyKey); k != "" {
		if len(k) > 64 {
			fail(c, http.StatusBadRequest, "invalid_body", "idempotency_key trop longue (64 caracteres max)")
			return
		}

		scoped := currentUser(c).ID.String() + ":" + k
		key = &scoped
	}

	user := currentUser(c)
	res, err := s.wallet.PlayRound(c.Request.Context(), g, wallet.PlayRequest{
		UserID:         user.ID,
		Amount:         in.Amount,
		Option:         in.Option,
		Params:         in.Params,
		IdempotencyKey: key,
	})
	switch {
	case errors.Is(err, wallet.ErrInvalidBet):
		fail(c, http.StatusBadRequest, "invalid_bet", err.Error())
		return
	case errors.Is(err, wallet.ErrInsufficientFunds):
		fail(c, http.StatusPaymentRequired, "insufficient_funds", "solde insuffisant pour cette mise")
		return
	case errors.Is(err, wallet.ErrUserNotFound):
		fail(c, http.StatusUnauthorized, "unauthenticated", "compte introuvable")
		return
	case err != nil:
		slog.Error("execution d'une mise", "err", err, "game", g.Key())
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"round":    roundResponse(res.Round, res.Detail),
		"balance":  res.NewBalance,
		"replayed": res.Replayed,
	})
}

type roundView struct {
	ID           string         `json:"id"`
	Game         string         `json:"game"`
	BetAmount    int64          `json:"bet_amount"`
	BetOption    string         `json:"bet_option"`
	Payout       int64          `json:"payout"`
	Net          int64          `json:"net"`
	BalanceAfter int64          `json:"balance_after"`
	Label        string         `json:"label"`
	Detail       map[string]any `json:"detail"`
	CreatedAt    string         `json:"created_at"`
}

func roundResponse(r models.Round, detail map[string]any) roundView {
	if detail == nil {
		detail = map[string]any{}
		if r.Detail != "" {
			_ = json.Unmarshal([]byte(r.Detail), &detail)
		}
	}
	return roundView{
		ID:           r.ID.String(),
		Game:         r.Game,
		BetAmount:    r.BetAmount,
		BetOption:    r.BetOption,
		Payout:       r.Payout,
		Net:          r.Net,
		BalanceAfter: r.BalanceAfter,
		Label:        r.Label,
		Detail:       detail,
		CreatedAt:    r.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
