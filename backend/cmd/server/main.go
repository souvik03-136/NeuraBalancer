// File: backend/cmd/server/main.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.uber.org/zap"

	"github.com/joho/godotenv"
)

// serverMetrics holds the latest sampled CPU and memory figures.
type serverMetrics struct {
	CPU    float64
	Memory float64
	mu     sync.RWMutex
}

func main() {
	_ = godotenv.Load()

	// ── Config from env ───────────────────────────────────────────────────────
	port := getEnv("BACKEND_PORT", "8001")
	instanceID := getEnv("BACKEND_INSTANCE_ID", "backend-"+port)
	logLevel := getEnv("LOG_LEVEL", "info")
	logFormat := getEnv("LOG_FORMAT", "json")

	// ── Logger ────────────────────────────────────────────────────────────────
	var logger *zap.Logger
	var err error
	if logFormat == "text" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init failed: %v\n", err)
		os.Exit(1)
	}
	_ = logLevel        // zap level configured above; extend if needed
	defer logger.Sync() //nolint:errcheck

	// ── Metrics loop ──────────────────────────────────────────────────────────
	sm := &serverMetrics{}
	go updateMetrics(sm, logger)

	// ── Routes ────────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":      "ok",
			"instance_id": instanceID,
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		sm.mu.RLock()
		defer sm.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]float64{
			"cpu_usage":    math.Round(sm.CPU*10) / 10,
			"memory_usage": math.Round(sm.Memory*10) / 10,
		})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"instance_id": instanceID,
			"port":        port,
		})
	})

	// ── HTTP Server ───────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("backend server listening",
			zap.String("port", port),
			zap.String("instance_id", instanceID),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// ── Graceful Shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("shutdown signal", zap.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Error("server error", zap.Error(err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}
	logger.Info("backend server stopped", zap.String("instance_id", instanceID))
}

// updateMetrics samples CPU and memory every 2 s with exponential smoothing.
func updateMetrics(sm *serverMetrics, logger *zap.Logger) {
	var smoothedCPU, smoothedMem float64
	first := true

	for {
		cpuPercents, err := cpu.Percent(2*time.Second, false)
		memStat, memErr := mem.VirtualMemory()
		if err != nil || memErr != nil || len(cpuPercents) == 0 {
			logger.Debug("metrics sample failed", zap.Error(err))
			time.Sleep(2 * time.Second)
			continue
		}

		curCPU := cpuPercents[0]
		curMem := memStat.UsedPercent

		if first {
			smoothedCPU = curCPU
			smoothedMem = curMem
			first = false
		} else {
			smoothedCPU = 0.7*smoothedCPU + 0.3*curCPU
			smoothedMem = 0.7*smoothedMem + 0.3*curMem
		}

		sm.mu.Lock()
		sm.CPU = clamp(smoothedCPU, 0, 100)
		sm.Memory = clamp(smoothedMem, 0, 100)
		sm.mu.Unlock()
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
