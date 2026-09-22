package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr           string
	TrustedProxies []string
	AllowedOrigins []string

	DatabaseURL string

	StartBalance int64
	SessionTTL   time.Duration
	CookieName   string
	CookieSecure bool
	CookieDomain string

	RateGlobalPerMin int
	RateAuthPerMin   int
	RatePlayPerMin   int

	MinBet int64
	MaxBet int64
}

func Load() (Config, error) {
	c := Config{
		Addr:             env("APP_ADDR", ":8080"),
		TrustedProxies:   list(env("TRUSTED_PROXIES", "")),
		AllowedOrigins:   list(env("ALLOWED_ORIGINS", "")),
		DatabaseURL:      env("DATABASE_URL", ""),
		StartBalance:     envInt64("START_BALANCE", 1000),
		SessionTTL:       envDuration("SESSION_TTL", 24*time.Hour),
		CookieName:       env("SESSION_COOKIE_NAME", "casino_session"),
		CookieSecure:     envBool("COOKIE_SECURE", true),
		CookieDomain:     env("COOKIE_DOMAIN", ""),
		RateGlobalPerMin: envInt("RATE_GLOBAL_PER_MIN", 300),
		RateAuthPerMin:   envInt("RATE_AUTH_PER_MIN", 10),
		RatePlayPerMin:   envInt("RATE_PLAY_PER_MIN", 120),
		MinBet:           envInt64("MIN_BET", 1),
		MaxBet:           envInt64("MAX_BET", 500),
	}

	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.MinBet < 1 {
		return c, fmt.Errorf("MIN_BET must be >= 1")
	}
	if c.MaxBet < c.MinBet {
		return c, fmt.Errorf("MAX_BET (%d) must be >= MIN_BET (%d)", c.MaxBet, c.MinBet)
	}
	return c, nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return def
}

func envInt64(key string, def int64) int64 {
	if v, err := strconv.ParseInt(env(key, ""), 10, 64); err == nil {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, err := strconv.ParseBool(env(key, "")); err == nil {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(env(key, "")); err == nil {
		return v
	}
	return def
}

func list(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
