package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/casino-rt/backend/internal/auth"
	"github.com/casino-rt/backend/internal/models"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

const (
	minPasswordLen = 8

	maxPasswordLen = 128
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Balance   int64  `json:"balance"`
	CreatedAt string `json:"created_at"`
}

func toUserResponse(u *models.User) userResponse {
	return userResponse{
		ID:        u.ID.String(),
		Username:  u.Username,
		Balance:   u.Balance,
		CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func (s *Server) handleRegister(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "corps de requete invalide")
		return
	}
	in.Username = strings.TrimSpace(in.Username)

	if !usernamePattern.MatchString(in.Username) {
		fail(c, http.StatusBadRequest, "invalid_username",
			"le pseudo doit faire 3 a 32 caracteres (lettres, chiffres, - et _)")
		return
	}
	if len(in.Password) < minPasswordLen || len(in.Password) > maxPasswordLen {
		fail(c, http.StatusBadRequest, "invalid_password",
			"le mot de passe doit faire entre 8 et 128 caracteres")
		return
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		slog.Error("hachage du mot de passe", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	user := models.User{
		Username:     in.Username,
		PasswordHash: hash,
		Balance:      s.cfg.StartBalance,
	}

	if err := s.db.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		if isDuplicate(err) {
			fail(c, http.StatusConflict, "username_taken", "ce pseudo est deja pris")
			return
		}
		slog.Error("creation du compte", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	s.issueSession(c, &user, http.StatusCreated)
}

func (s *Server) handleLogin(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "corps de requete invalide")
		return
	}
	in.Username = strings.TrimSpace(in.Username)

	var user models.User
	err := s.db.WithContext(c.Request.Context()).
		Where("username = ?", in.Username).
		Take(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		auth.DummyVerify(in.Password)
		fail(c, http.StatusUnauthorized, "invalid_credentials", "pseudo ou mot de passe incorrect")
		return
	}
	if err != nil {
		slog.Error("lecture du compte", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	if err := auth.VerifyPassword(in.Password, user.PasswordHash); err != nil {
		fail(c, http.StatusUnauthorized, "invalid_credentials", "pseudo ou mot de passe incorrect")
		return
	}

	s.issueSession(c, &user, http.StatusOK)
}

func (s *Server) issueSession(c *gin.Context, user *models.User, status int) {
	token, expires, err := s.sessions.Create(
		c.Request.Context(), user.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		slog.Error("creation de session", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	s.setSessionCookie(c, token, expires)
	c.JSON(status, gin.H{
		"user":       toUserResponse(user),
		"expires_at": expires.UTC(),
	})
}

func (s *Server) handleLogout(c *gin.Context) {
	if token, err := c.Cookie(s.cfg.CookieName); err == nil && token != "" {
		if err := s.sessions.Destroy(c.Request.Context(), token); err != nil {
			slog.Error("suppression de session", "err", err)
		}
	}
	s.clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "deconnecte"})
}

func (s *Server) handleMe(c *gin.Context) {
	user := currentUser(c)

	balance, err := s.wallet.Balance(c.Request.Context(), user.ID)
	if err != nil {
		slog.Error("lecture du solde", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	user.Balance = balance
	c.JSON(http.StatusOK, gin.H{"user": toUserResponse(user)})
}

func isDuplicate(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
