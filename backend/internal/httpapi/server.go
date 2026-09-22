package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/casino-rt/backend/internal/auth"
	"github.com/casino-rt/backend/internal/config"
	"github.com/casino-rt/backend/internal/wallet"
)

type Server struct {
	cfg      config.Config
	db       *gorm.DB
	sessions *auth.SessionStore
	wallet   *wallet.Service
}

func NewServer(cfg config.Config, db *gorm.DB, sessions *auth.SessionStore, w *wallet.Service) *Server {
	return &Server{cfg: cfg, db: db, sessions: sessions, wallet: w}
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), requestLogger())

	if len(s.cfg.TrustedProxies) > 0 {
		_ = r.SetTrustedProxies(s.cfg.TrustedProxies)
	} else {
		_ = r.SetTrustedProxies(nil)
	}
	r.RemoteIPHeaders = []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"}

	if len(s.cfg.AllowedOrigins) > 0 {
		r.Use(corsMiddleware(s.cfg.AllowedOrigins))
	}

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", s.handleReady)

	globalLimiter := newIPLimiter(s.cfg.RateGlobalPerMin, 5*time.Minute)

	authLimiter := newIPLimiter(s.cfg.RateAuthPerMin, 15*time.Minute)

	playLimiter := newKeyLimiter(s.cfg.RatePlayPerMin, 15*time.Minute)

	api := r.Group("/api", globalLimiter.middleware())

	api.GET("/config", s.handleConfig)

	authGroup := api.Group("/auth")
	authGroup.POST("/register", authLimiter.middleware(), s.handleRegister)
	authGroup.POST("/login", authLimiter.middleware(), s.handleLogin)
	authGroup.POST("/logout", s.handleLogout)

	api.GET("/games", s.handleListGames)
	api.GET("/games/:key", s.handleGetGame)

	api.GET("/leaderboard", s.handleLeaderboard)

	private := api.Group("", s.requireAuth())
	private.GET("/me", s.handleMe)
	private.GET("/me/history", s.handleHistory)
	private.GET("/me/stats", s.handleStats)
	private.POST("/games/:key/play", playLimiter.middlewareByUser(), s.handlePlay)

	r.NoRoute(func(c *gin.Context) {
		fail(c, http.StatusNotFound, "not_found", "route inconnue")
	})
	return r
}

func (s *Server) handleReady(c *gin.Context) {
	sqlDB, err := s.db.DB()
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "db_unavailable", "base de donnees indisponible")
		return
	}
	ctx, cancel := contextWithTimeout(c, 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		fail(c, http.StatusServiceUnavailable, "db_unavailable", "base de donnees indisponible")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (s *Server) handleConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"min_bet":       s.cfg.MinBet,
		"max_bet":       s.cfg.MaxBet,
		"start_balance": s.cfg.StartBalance,
		"currency":      "credits fictifs",
	})
}
