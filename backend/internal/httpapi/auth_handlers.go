package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/casino-rt/backend/internal/models"
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

	user, err := s.createAccount(c.Request.Context(), in.Username, in.Password)
	switch {
	case errors.Is(err, errInvalidUsername):
		fail(c, http.StatusBadRequest, "invalid_username", err.Error())
		return
	case errors.Is(err, errInvalidPassword):
		fail(c, http.StatusBadRequest, "invalid_password", err.Error())
		return
	case errors.Is(err, errUsernameTaken):
		fail(c, http.StatusConflict, "username_taken", err.Error())
		return
	case err != nil:
		slog.Error("inscription", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	s.issueSession(c, user, http.StatusCreated)
}

func (s *Server) handleLogin(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "invalid_body", "corps de requete invalide")
		return
	}

	user, err := s.authenticate(c.Request.Context(), in.Username, in.Password)
	switch {
	case errors.Is(err, errInvalidCredentials):
		fail(c, http.StatusUnauthorized, "invalid_credentials", err.Error())
		return
	case err != nil:
		slog.Error("connexion", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}

	s.issueSession(c, user, http.StatusOK)
}

func (s *Server) issueSession(c *gin.Context, user *models.User, status int) {
	expires, err := s.openSession(c, user)
	if err != nil {
		slog.Error("ouverture de session", "err", err)
		fail(c, http.StatusInternalServerError, "internal", "erreur interne")
		return
	}
	c.JSON(status, gin.H{
		"user":       toUserResponse(user),
		"expires_at": expires.UTC(),
	})
}

func (s *Server) handleLogout(c *gin.Context) {
	if err := s.closeSession(c); err != nil {
		slog.Error("suppression de session", "err", err)
	}
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
