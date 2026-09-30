package httpapi

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/games/roulette"
	"github.com/casino-rt/backend/internal/games/slots"
	"github.com/casino-rt/backend/internal/models"
	"github.com/casino-rt/backend/internal/wallet"
	"github.com/casino-rt/backend/internal/web/views"
)

const historyPageSize = 20

func (s *Server) pageLobby(c *gin.Context) {
	top, err := s.loadLeaderboard(c.Request.Context(), "balance", 5)
	if err != nil {
		slog.Error("classement de l'accueil", "err", err)
	}
	render(c, http.StatusOK, views.Lobby(s.page(c, "", "jeux"), s.gameCards(), s.leaderRows(c, top), s.cfg.StartBalance))
}

func (s *Server) gameCards() []views.GameCard {
	all := games.All()
	cards := make([]views.GameCard, 0, len(all))
	for _, g := range all {
		lo, hi := s.wallet.Bounds(g)
		card := views.GameCard{
			Key:         g.Key(),
			Name:        g.Name(),
			Description: g.Description(),
			MinBet:      lo,
			MaxBet:      hi,
		}
		switch g := g.(type) {
		case *slots.Machine:
			sg := views.NewSlotsGame(g.Definition(), lo, hi)
			card.Kind, card.RTP, card.Volatility = "slots", sg.RTP, sg.Volatility
			card.Bg, card.Accent = sg.Bg, sg.Accent
			for _, row := range sg.Pays[:min(3, len(sg.Pays))] {
				card.Preview = append(card.Preview, row.Symbol.Emoji)
			}
		case *roulette.Game:
			card.Kind, card.RTP = "roulette", 36.0/37.0
			card.Bg, card.Accent = "#0b2a1f", "#c0392b"
		default:
			continue
		}
		cards = append(cards, card)
	}
	return cards
}

func (s *Server) leaderRows(c *gin.Context, board []leaderEntry) []views.LeaderRow {
	me := ""
	if u := currentUser(c); u != nil {
		me = u.Username
	}
	rows := make([]views.LeaderRow, 0, len(board))
	for _, e := range board {
		rows = append(rows, views.LeaderRow{
			Rank:       e.Rank,
			Username:   e.Username,
			Balance:    e.Balance,
			Rounds:     e.Rounds,
			Net:        e.Net,
			BiggestWin: e.BiggestWin,
			Me:         e.Username == me,
		})
	}
	return rows
}

func (s *Server) pageLeaderboard(c *gin.Context) {
	metric := c.DefaultQuery("metric", "balance")
	board, err := s.loadLeaderboard(c.Request.Context(), metric, 50)
	if errors.Is(err, errInvalidMetric) {
		metric = "balance"
		board, err = s.loadLeaderboard(c.Request.Context(), metric, 50)
	}
	if err != nil {
		slog.Error("classement", "err", err)
		s.pageError(c, http.StatusInternalServerError, "Le classement est indisponible.")
		return
	}
	render(c, http.StatusOK, views.LeaderboardPage(s.page(c, "Classement", "classement"), metric, s.leaderRows(c, board)))
}

func (s *Server) pageAuth(register bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		next := safeNext(c.Query("suite"))
		if currentUser(c) != nil {
			redirect(c, cmp.Or(next, "/"))
			return
		}
		s.renderAuth(c, http.StatusOK, views.AuthForm{Register: register, Next: next})
	}
}

func (s *Server) renderAuth(c *gin.Context, status int, f views.AuthForm) {
	title := "Connexion"
	if f.Register {
		title = "Inscription"
	}
	f.Start = s.cfg.StartBalance
	render(c, status, views.AuthPage(s.page(c, title, ""), f))
}

func (s *Server) submitLogin(c *gin.Context) {
	f := views.AuthForm{Username: c.PostForm("username"), Next: safeNext(c.PostForm("suite"))}
	if !s.limits.auth.allow(c.ClientIP()) {
		f.Error = "Trop de tentatives, réessayez dans quelques minutes."
		s.renderAuth(c, http.StatusTooManyRequests, f)
		return
	}

	user, err := s.authenticate(c.Request.Context(), f.Username, c.PostForm("password"))
	switch {
	case errors.Is(err, errInvalidCredentials):
		f.Error = sentence(err.Error())
		s.renderAuth(c, http.StatusUnauthorized, f)
		return
	case err != nil:
		slog.Error("connexion", "err", err)
		f.Error = "Erreur interne, réessayez."
		s.renderAuth(c, http.StatusInternalServerError, f)
		return
	}
	s.startPageSession(c, user, f)
}

func (s *Server) submitRegister(c *gin.Context) {
	f := views.AuthForm{Register: true, Username: c.PostForm("username"), Next: safeNext(c.PostForm("suite"))}
	if !s.limits.auth.allow(c.ClientIP()) {
		f.Error = "Trop de tentatives, réessayez dans quelques minutes."
		s.renderAuth(c, http.StatusTooManyRequests, f)
		return
	}

	user, err := s.createAccount(c.Request.Context(), f.Username, c.PostForm("password"))
	switch {
	case errors.Is(err, errInvalidUsername), errors.Is(err, errInvalidPassword):
		f.Error = sentence(err.Error())
		s.renderAuth(c, http.StatusBadRequest, f)
		return
	case errors.Is(err, errUsernameTaken):
		f.Error = sentence(err.Error())
		s.renderAuth(c, http.StatusConflict, f)
		return
	case err != nil:
		slog.Error("inscription", "err", err)
		f.Error = "Erreur interne, réessayez."
		s.renderAuth(c, http.StatusInternalServerError, f)
		return
	}
	s.startPageSession(c, user, f)
}

func (s *Server) startPageSession(c *gin.Context, user *models.User, f views.AuthForm) {
	if _, err := s.openSession(c, user); err != nil {
		slog.Error("ouverture de session", "err", err)
		f.Error = "Erreur interne, réessayez."
		s.renderAuth(c, http.StatusInternalServerError, f)
		return
	}
	redirect(c, cmp.Or(f.Next, "/"))
}

func (s *Server) submitLogout(c *gin.Context) {
	if err := s.closeSession(c); err != nil {
		slog.Error("suppression de session", "err", err)
	}
	redirect(c, "/")
}

func (s *Server) pageProfile(c *gin.Context) {
	user := currentUser(c)
	ctx := c.Request.Context()
	page := intQuery(c, "page", 1, 1, 10000)

	stats, err := s.loadStats(ctx, user.ID)
	if err != nil {
		slog.Error("statistiques", "err", err)
		s.pageError(c, http.StatusInternalServerError, "Le profil est indisponible.")
		return
	}
	rounds, total, err := s.loadHistory(ctx, user.ID, "", historyPageSize, (page-1)*historyPageSize)
	if err != nil {
		slog.Error("historique", "err", err)
		s.pageError(c, http.StatusInternalServerError, "Le profil est indisponible.")
		return
	}

	pr := views.Profile{
		Balance: stats.Balance,
		Total:   views.Totals(stats.Total),
		Page:    page,
		Pages:   int((total + historyPageSize - 1) / historyPageSize),
	}
	for _, g := range stats.PerGame {
		pr.PerGame = append(pr.PerGame, views.GameStat{
			Name:    gameName(g.Game),
			Rounds:  g.Rounds,
			Wagered: g.Wagered,
			Net:     g.Net,
			WinRate: g.WinRate,
			BestWin: g.BestWin,
		})
	}
	for _, r := range rounds {
		pr.History = append(pr.History, views.HistoryRow{
			When:         r.CreatedAt.In(time.Local).Format("02/01 15:04"),
			Game:         gameName(r.Game),
			Bet:          r.BetAmount,
			Label:        r.Label,
			Net:          r.Net,
			BalanceAfter: r.BalanceAfter,
		})
	}

	p := s.page(c, "Profil", "profil")
	p.User.Balance = stats.Balance
	render(c, http.StatusOK, views.ProfilePage(p, pr))
}

func gameName(key string) string {
	if g, err := games.Get(key); err == nil {
		return g.Name()
	}
	return key
}

// stage is what a game page shows: its view, the balance chip that goes with
// it, and the full page when htmx is not doing the swap.
type stage struct {
	swap  func(views.User) templ.Component
	page  func(views.Page) templ.Component
	delay float64
}

func (s *Server) gameStage(g games.Game, res *wallet.PlayResult, amount int64, option, errMsg string) (stage, bool) {
	lo, hi := s.wallet.Bounds(g)
	if amount < lo || amount > hi {
		amount = min(max(10, lo), hi)
	}
	token := uuid.NewString()

	switch g := g.(type) {
	case *slots.Machine:
		sg := views.NewSlotsGame(g.Definition(), lo, hi)
		st := views.SlotsIdle(sg, amount, token)
		if res != nil {
			st = views.SlotsResult(sg, res.Detail, res.Round.BetAmount, res.Round.Payout, res.Round.Label, amount, token)
		}
		st.Error = errMsg
		return stage{
			swap:  func(u views.User) templ.Component { return views.SlotsSwap(st, u) },
			page:  func(p views.Page) templ.Component { return views.SlotsPage(p, st) },
			delay: st.Settle(),
		}, true
	case *roulette.Game:
		rg := views.NewRouletteGame(g.Key(), g.Name(), g.Description(), lo, hi)
		st := views.RouletteIdle(rg, amount, option, token)
		if res != nil {
			st = views.RouletteResult(rg, res.Detail, res.Round.BetAmount, res.Round.Payout, option, amount, token)
		}
		st.Error = errMsg
		return stage{
			swap:  func(u views.User) templ.Component { return views.RouletteSwap(st, u) },
			page:  func(p views.Page) templ.Component { return views.RoulettePage(p, st) },
			delay: st.Settle(),
		}, true
	}
	return stage{}, false
}

func (s *Server) pageGame(c *gin.Context) {
	g, err := games.Get(c.Param("key"))
	if err != nil {
		s.pageError(c, http.StatusNotFound, "Ce jeu n'existe pas.")
		return
	}
	st, ok := s.gameStage(g, nil, 0, "", "")
	if !ok {
		s.pageError(c, http.StatusNotFound, "Ce jeu n'a pas encore d'interface.")
		return
	}
	render(c, http.StatusOK, st.page(s.page(c, g.Name(), "jeux")))
}

func (s *Server) submitPlay(c *gin.Context) {
	g, err := games.Get(c.Param("key"))
	if err != nil {
		s.pageError(c, http.StatusNotFound, "Ce jeu n'existe pas.")
		return
	}
	user := currentUser(c)
	amount, _ := strconv.ParseInt(strings.TrimSpace(c.PostForm("amount")), 10, 64)
	option := strings.TrimSpace(c.PostForm("option"))

	var res *wallet.PlayResult
	status, errMsg := http.StatusOK, ""
	if !s.limits.play.allow("user:" + user.ID.String()) {
		status, errMsg = http.StatusTooManyRequests, "Trop de mises envoyées, ralentissez."
	} else {
		req := wallet.PlayRequest{UserID: user.ID, Amount: amount, Option: option}
		if token := strings.TrimSpace(c.PostForm("token")); token != "" && len(token) <= 64 {
			key := user.ID.String() + ":" + token
			req.IdempotencyKey = &key
		}
		res, err = s.wallet.PlayRound(c.Request.Context(), g, req)
		switch {
		case errors.Is(err, wallet.ErrInvalidBet):
			status, errMsg = http.StatusUnprocessableEntity, sentence(strings.TrimPrefix(err.Error(), wallet.ErrInvalidBet.Error()+": "))
		case errors.Is(err, wallet.ErrInsufficientFunds):
			status, errMsg = http.StatusPaymentRequired, "Solde insuffisant pour cette mise."
		case errors.Is(err, wallet.ErrUserNotFound):
			s.clearSessionCookie(c)
			redirect(c, "/connexion")
			return
		case err != nil:
			slog.Error("execution d'une mise", "err", err, "game", g.Key())
			status, errMsg = http.StatusInternalServerError, "Erreur interne, réessayez."
		}
	}

	st, ok := s.gameStage(g, res, amount, option, errMsg)
	if !ok {
		s.pageError(c, http.StatusNotFound, "Ce jeu n'a pas encore d'interface.")
		return
	}

	u := views.User{Name: user.Username, Balance: user.Balance}
	if res != nil {
		u.Balance = res.NewBalance
		u.Previous = res.Round.BalanceAfter - res.Round.Net
		u.Delay = st.delay
	}

	if isHTMX(c) {
		// htmx only swaps 2xx responses; the error is part of the stage.
		render(c, http.StatusOK, st.swap(u))
		return
	}
	p := s.page(c, g.Name(), "jeux")
	p.User = &u
	render(c, status, st.page(p))
}
