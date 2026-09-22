package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/casino-rt/backend/internal/models"
)

var ErrNoSession = errors.New("unknown or expired session")

type SessionStore struct {
	db  *gorm.DB
	ttl time.Duration
}

func NewSessionStore(db *gorm.DB, ttl time.Duration) *SessionStore {
	return &SessionStore{db: db, ttl: ttl}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *SessionStore) Create(ctx context.Context, userID uuid.UUID, ip, userAgent string) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().Add(s.ttl)

	if len(userAgent) > 255 {
		userAgent = userAgent[:255]
	}
	sess := models.Session{
		TokenHash: hashToken(token),
		UserID:    userID,
		IP:        ip,
		UserAgent: userAgent,
		ExpiresAt: expires,
	}
	if err := s.db.WithContext(ctx).Create(&sess).Error; err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *SessionStore) Resolve(ctx context.Context, token string) (*models.User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	var sess models.Session
	err := s.db.WithContext(ctx).
		Preload("User").
		Where("token_hash = ? AND expires_at > ?", hashToken(token), time.Now()).
		Take(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	if sess.User.ID == uuid.Nil {
		return nil, ErrNoSession
	}
	return &sess.User, nil
}

func (s *SessionStore) Destroy(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.db.WithContext(ctx).
		Where("token_hash = ?", hashToken(token)).
		Delete(&models.Session{}).Error
}

func (s *SessionStore) DestroyAllForUser(ctx context.Context, userID uuid.UUID) error {
	return s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&models.Session{}).Error
}

func (s *SessionStore) StartCleanup(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.db.WithContext(ctx).
					Where("expires_at < ?", time.Now()).
					Delete(&models.Session{})
			}
		}
	}()
}
