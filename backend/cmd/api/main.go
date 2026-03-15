// File: backend/cmd/api/main.go
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/api"
	"github.com/souvik03-136/neurabalancer/backend/internal/config"
	"github.com/souvik03-136/neurabalancer/backend/internal/database"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
	"github.com/souvik03-136/neurabalancer/backend/internal/tracer"
)

func main() {
	// ── Config ────────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		// logger not ready yet, use stderr
		_, _ = os.Stderr.WriteString("FATAL config error: " + err.Error() + "\n")
		os.Exit(1)
	}

	// ── Logger ────────────────────────────────────────────────────────────────
	logger, err := config.NewLogger(cfg.App.LogLevel, cfg.App.LogFormat)
	if err != nil {
		_, _ = os.Stderr.WriteString("FATAL logger init: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck

	logger.Info("starting neurabalancer",
		zap.String("env", cfg.App.Env),
		zap.String("strategy", cfg.LB.Strategy),
		zap.Strings("servers", cfg.LB.Servers),
	)

	// ── Tracing ───────────────────────────────────────────────────────────────
	var tp *tracer.Provider
	if cfg.Telemetry.OTELEnabled {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		tp, err = tracer.Init(ctx, cfg.Telemetry.ServiceName, cfg.Telemetry.OTLPEndpoint)
		if err != nil {
			logger.Warn("tracing init failed, continuing without traces", zap.Error(err))
			tp = tracer.NoopProvider()
		} else {
			logger.Info("tracing initialised", zap.String("endpoint", cfg.Telemetry.OTLPEndpoint))
		}
	} else {
		tp = tracer.NoopProvider()
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutCtx); err != nil {
			logger.Warn("tracer shutdown error", zap.Error(err))
		}
	}()

	// ── Database ──────────────────────────────────────────────────────────────
	db, err := database.New(&cfg.Database, logger)
	if err != nil {
		logger.Fatal("database connection failed", zap.Error(err))
	}
	defer db.Close()

	// ── Metrics Collector ─────────────────────────────────────────────────────
	col := metrics.NewCollector(db, logger)

	// ── Strategy ──────────────────────────────────────────────────────────────
	strategy, err := loadbalancer.NewStrategy(cfg.LB.Strategy, &cfg.ML, col, logger)
	if err != nil {
		logger.Fatal("strategy init failed", zap.Error(err))
	}

	// ── Load Balancer ─────────────────────────────────────────────────────────
	lb := loadbalancer.New(strategy, cfg.LB.Servers, &cfg.Health, db, col, logger)
	defer lb.Stop()

	// ── HTTP Server ───────────────────────────────────────────────────────────
	router := api.NewRouter(lb, logger, cfg.Telemetry.ServiceName)
	srv := &http.Server{
		Addr:         cfg.Server.Addr(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in background goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", zap.String("addr", cfg.Server.Addr()))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// ── Graceful Shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Error("server error", zap.Error(err))
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	} else {
		logger.Info("server shutdown complete")
	}
}
