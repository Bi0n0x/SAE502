package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"

	"github.com/casino-rt/backend/internal/auth"
	"github.com/casino-rt/backend/internal/web"
	"github.com/casino-rt/backend/internal/web/views"
)

const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

func (s *Server) mountPages(r *gin.Engine) {
	files := http.StripPrefix("/static/", http.FileServerFS(web.Static))
	r.GET("/static/*path", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		files.ServeHTTP(c.Writer, c.Request)
	})

	pages := r.Group("", s.pageHeaders(), s.pageLimit(s.limits.global), s.sameOrigin(), s.loadUser())
	pages.GET("/", s.pageLobby)
	pages.GET("/classement", s.pageLeaderboard)
	pages.GET("/connexion", s.pageAuth(false))
	pages.POST("/connexion", s.submitLogin)
	pages.GET("/inscription", s.pageAuth(true))
	pages.POST("/inscription", s.submitRegister)
	pages.POST("/deconnexion", s.submitLogout)

	private := pages.Group("", s.requirePageUser())
	private.GET("/profil", s.pageProfile)
	private.GET("/jeux/:key", s.pageGame)
	private.POST("/jeux/:key", s.submitPlay)
}

func render(c *gin.Context, status int, page templ.Component) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	if err := page.Render(c.Request.Context(), c.Writer); err != nil {
		slog.Error("rendu de page", "err", err, "path", c.Request.URL.Path)
	}
}

func (s *Server) page(c *gin.Context, title, active string) views.Page {
	p := views.Page{Title: title, Active: active}
	if u := currentUser(c); u != nil {
		p.User = &views.User{Name: u.Username, Balance: u.Balance}
	}
	return p
}

func (s *Server) pageError(c *gin.Context, status int, message string) {
	render(c, status, views.ErrorPage(s.page(c, "Erreur", ""), status, message))
	c.Abort()
}

func isHTMX(c *gin.Context) bool { return c.GetHeader("HX-Request") == "true" }

// redirect sends the browser elsewhere; htmx needs a header instead of a 303,
// otherwise it would swap the target page into the game stage.
func redirect(c *gin.Context, target string) {
	if isHTMX(c) {
		c.Header("HX-Redirect", target)
		c.Status(http.StatusOK)
	} else {
		c.Redirect(http.StatusSeeOther, target)
	}
	c.Abort()
}

func (s *Server) pageHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("Vary", "HX-Request")
		c.Next()
	}
}

func (s *Server) loadUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(s.cfg.CookieName)
		if err != nil || token == "" {
			c.Next()
			return
		}
		user, err := s.sessions.Resolve(c.Request.Context(), token)
		switch {
		case err == nil:
			c.Set(contextUserKey, user)
		case errors.Is(err, auth.ErrNoSession):
			s.clearSessionCookie(c)
		default:
			slog.Error("resolution de session", "err", err)
		}
		c.Next()
	}
}

func (s *Server) requirePageUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentUser(c) != nil {
			c.Next()
			return
		}
		next := c.Request.URL.RequestURI()
		if c.Request.Method != http.MethodGet {
			next = c.Request.URL.Path
		}
		redirect(c, "/connexion?suite="+url.QueryEscape(next))
	}
}

func (s *Server) pageLimit(k *keyLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !k.allow(c.ClientIP()) {
			s.pageError(c, http.StatusTooManyRequests, "Trop de requêtes, réessayez dans un instant.")
			return
		}
		c.Next()
	}
}

// sameOrigin rejects form posts coming from another site. The session cookie
// is SameSite=Lax already; this also covers older browsers.
func (s *Server) sameOrigin() gin.HandlerFunc {
	guard := http.NewCrossOriginProtection()
	return func(c *gin.Context) {
		if err := guard.Check(c.Request); err != nil {
			s.pageError(c, http.StatusForbidden, "Requête refusée.")
			return
		}
		c.Next()
	}
}

// safeNext keeps only local paths, so ?suite= cannot send people off-site.
func safeNext(raw string) string {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host != "" || u.Scheme != "" {
		return ""
	}
	return raw
}

func sentence(msg string) string {
	r, size := utf8.DecodeRuneInString(msg)
	if r == utf8.RuneError {
		return msg
	}
	msg = string(unicode.ToUpper(r)) + msg[size:]
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return msg
}
