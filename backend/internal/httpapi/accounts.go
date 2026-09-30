package httpapi

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

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

var (
	errInvalidUsername    = errors.New("le pseudo doit faire 3 a 32 caracteres (lettres, chiffres, - et _)")
	errInvalidPassword    = errors.New("le mot de passe doit faire entre 8 et 128 caracteres")
	errUsernameTaken      = errors.New("ce pseudo est deja pris")
	errInvalidCredentials = errors.New("pseudo ou mot de passe incorrect")
)

func (s *Server) createAccount(ctx context.Context, username, password string) (*models.User, error) {
	username = strings.TrimSpace(username)
	if !usernamePattern.MatchString(username) {
		return nil, errInvalidUsername
	}
	if len(password) < minPasswordLen || len(password) > maxPasswordLen {
		return nil, errInvalidPassword
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hachage du mot de passe: %w", err)
	}

	user := models.User{
		Username:     username,
		PasswordHash: hash,
		Balance:      s.cfg.StartBalance,
	}
	if err := s.db.WithContext(ctx).Create(&user).Error; err != nil {
		if isDuplicate(err) {
			return nil, errUsernameTaken
		}
		return nil, fmt.Errorf("creation du compte: %w", err)
	}
	return &user, nil
}

func (s *Server) authenticate(ctx context.Context, username, password string) (*models.User, error) {
	var user models.User
	err := s.db.WithContext(ctx).
		Where("username = ?", strings.TrimSpace(username)).
		Take(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		auth.DummyVerify(password)
		return nil, errInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("lecture du compte: %w", err)
	}
	if err := auth.VerifyPassword(password, user.PasswordHash); err != nil {
		return nil, errInvalidCredentials
	}
	return &user, nil
}

func (s *Server) openSession(c *gin.Context, user *models.User) (time.Time, error) {
	token, expires, err := s.sessions.Create(
		c.Request.Context(), user.ID, c.ClientIP(), c.GetHeader("User-Agent"))
	if err != nil {
		return time.Time{}, fmt.Errorf("creation de session: %w", err)
	}
	s.setSessionCookie(c, token, expires)
	return expires, nil
}

func (s *Server) closeSession(c *gin.Context) error {
	defer s.clearSessionCookie(c)
	if token, err := c.Cookie(s.cfg.CookieName); err == nil && token != "" {
		return s.sessions.Destroy(c.Request.Context(), token)
	}
	return nil
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
