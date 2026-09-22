package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/casino-rt/backend/internal/auth"
	"github.com/casino-rt/backend/internal/config"
	"github.com/casino-rt/backend/internal/db"
	"github.com/casino-rt/backend/internal/games"
	"github.com/casino-rt/backend/internal/games/catalog"
	"github.com/casino-rt/backend/internal/httpapi"
	"github.com/casino-rt/backend/internal/rng"
	"github.com/casino-rt/backend/internal/wallet"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := run(); err != nil {
		slog.Error("arret du service", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	catalog.Register()
	for _, g := range games.All() {
		slog.Info("jeu charge", "key", g.Key(), "name", g.Name())
	}

	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if err := db.Migrate(gdb); err != nil {
		return err
	}
	slog.Info("base de donnees prete")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sessions := auth.NewSessionStore(gdb, cfg.SessionTTL)
	sessions.StartCleanup(ctx, time.Hour)

	purse := wallet.New(gdb, rng.Crypto{}, cfg.MinBet, cfg.MaxBet)

	gin.SetMode(gin.ReleaseMode)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           httpapi.NewServer(cfg, gdb, sessions, purse).Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("ecoute HTTP", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():

		slog.Info("signal d'arret recu, fermeture en cours")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
		slog.Info("service arrete")
		return nil
	}
}
