package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"github.com/casino-rt/backend/internal/auth"
	"github.com/casino-rt/backend/internal/models"
)

const contextUserKey = "casino.user"

func fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

func contextWithTimeout(c *gin.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), d)
}

func currentUser(c *gin.Context) *models.User {
	v, ok := c.Get(contextUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*models.User)
	return u
}

func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(s.cfg.CookieName)
		if err != nil || token == "" {
			fail(c, http.StatusUnauthorized, "unauthenticated", "connexion requise")
			return
		}
		user, err := s.sessions.Resolve(c.Request.Context(), token)
		if errors.Is(err, auth.ErrNoSession) {
			s.clearSessionCookie(c)
			fail(c, http.StatusUnauthorized, "session_expired", "session expiree, reconnectez-vous")
			return
		}
		if err != nil {
			slog.Error("resolution de session", "err", err)
			fail(c, http.StatusInternalServerError, "internal", "erreur interne")
			return
		}
		c.Set(contextUserKey, user)
		c.Next()
	}
}

func (s *Server) setSessionCookie(c *gin.Context, token string, expires time.Time) {
	c.SetSameSite(http.SameSiteLaxMode)
	maxAge := int(time.Until(expires).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	c.SetCookie(s.cfg.CookieName, token, maxAge, "/", s.cfg.CookieDomain, s.cfg.CookieSecure, true)
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(s.cfg.CookieName, "", -1, "/", s.cfg.CookieDomain, s.cfg.CookieSecure, true)
}

func corsMiddleware(allowed []string) gin.HandlerFunc {
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		set[strings.TrimRight(o, "/")] = true
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if origin != "" && set[origin] {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Headers", "Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		level := slog.LevelInfo
		if c.Writer.Status() >= 500 {
			level = slog.LevelError
		}
		slog.Log(c.Request.Context(), level, "requete",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duree_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}

type keyLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	rate     rate.Limit
	burst    int
}

type entry struct {
	lim  *rate.Limiter
	seen time.Time
}

func newKeyLimiter(perMinute int, ttl time.Duration) *keyLimiter {
	if perMinute <= 0 {
		perMinute = 60
	}
	k := &keyLimiter{
		limiters: make(map[string]*entry),
		rate:     rate.Limit(float64(perMinute) / 60.0),
		burst:    max(perMinute/6, 5),
	}
	go k.cleanup(ttl)
	return k
}

func newIPLimiter(perMinute int, ttl time.Duration) *keyLimiter {
	return newKeyLimiter(perMinute, ttl)
}

func (k *keyLimiter) allow(key string) bool {
	k.mu.Lock()
	e, ok := k.limiters[key]
	if !ok {
		e = &entry{lim: rate.NewLimiter(k.rate, k.burst)}
		k.limiters[key] = e
	}
	e.seen = time.Now()
	k.mu.Unlock()
	return e.lim.Allow()
}

func (k *keyLimiter) cleanup(ttl time.Duration) {
	ticker := time.NewTicker(ttl)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-ttl)
		k.mu.Lock()
		for key, e := range k.limiters {
			if e.seen.Before(cutoff) {
				delete(k.limiters, key)
			}
		}
		k.mu.Unlock()
	}
}

func (k *keyLimiter) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !k.allow(c.ClientIP()) {
			fail(c, http.StatusTooManyRequests, "rate_limited", "trop de requetes, reessayez dans un instant")
			return
		}
		c.Next()
	}
}

func (k *keyLimiter) middlewareByUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := currentUser(c)
		key := c.ClientIP()
		if user != nil {
			key = "user:" + user.ID.String()
		}
		if !k.allow(key) {
			fail(c, http.StatusTooManyRequests, "rate_limited", "trop de mises envoyees, ralentissez")
			return
		}
		c.Next()
	}
}
