package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Username     string    `gorm:"size:32;uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Balance      int64     `gorm:"not null;default:1000;check:chk_users_balance_positive,balance >= 0" json:"balance"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(*gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}

type Session struct {
	TokenHash string    `gorm:"size:64;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;index;not null"`
	User      User      `gorm:"constraint:OnDelete:CASCADE"`
	IP        string    `gorm:"size:64"`
	UserAgent string    `gorm:"size:255"`
	ExpiresAt time.Time `gorm:"index;not null"`
	CreatedAt time.Time
}

type Round struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index:idx_rounds_user_time,priority:1" json:"user_id"`
	Game         string    `gorm:"size:48;not null;index" json:"game"`
	BetAmount    int64     `gorm:"not null" json:"bet_amount"`
	BetOption    string    `gorm:"size:64" json:"bet_option"`
	Payout       int64     `gorm:"not null" json:"payout"`
	Net          int64     `gorm:"not null" json:"net"`
	BalanceAfter int64     `gorm:"not null" json:"balance_after"`
	Label        string    `gorm:"size:120" json:"label"`

	Detail string `gorm:"type:jsonb;not null;default:'{}'" json:"-"`

	IdempotencyKey *string   `gorm:"size:128;uniqueIndex" json:"-"`
	CreatedAt      time.Time `gorm:"index:idx_rounds_user_time,priority:2,sort:desc" json:"created_at"`
}

func (r *Round) BeforeCreate(*gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

func AllModels() []any {
	return []any{&User{}, &Session{}, &Round{}}
}
