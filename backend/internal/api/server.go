package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type ServerMetrics struct {
	CPU    float64
	Memory float64
	mu     sync.RWMutex
}

func StartBackendServer(serverAddr string) {
	parsed, err := url.Parse(serverAddr)
	if err != nil {
		log.Fatalf("❌ Invalid server URL: %s, error: %v", serverAddr, err)
	}

	port := parsed.Port()
	if port == "" {
		port = "80"
	}

	// Initialize metrics
	metrics := &ServerMetrics{}
	go updateMetrics(metrics)

	// Create the mux router
	mux := http.NewServeMux()

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.mu.RLock()
		defer metrics.mu.RUnlock()

		json.NewEncoder(w).Encode(map[string]float64{
			"cpu_usage":    math.Round(metrics.CPU*10) / 10,
			"memory_usage": math.Round(metrics.Memory*10) / 10,
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from server on port %s", port)
	})

	// Start server
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("🚀 Backend server starting on port %s...\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Backend server failed on port %s: %v", port, err)
		}
	}()

	<-shutdownChan
	log.Printf("🛑 Shutting down server on port %s...\n", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Server on port %s forced to shutdown: %v", port, err)
	}

	log.Printf("✅ Server on port %s exited cleanly\n", port)
}

func updateMetrics(metrics *ServerMetrics) {
	var smoothedCPU, smoothedMemory float64

	for {
		cpuPercents, err := cpu.Percent(2*time.Second, false)
		memStat, err := mem.VirtualMemory()

		if err == nil && len(cpuPercents) > 0 {
			currentCPU := cpuPercents[0]
			currentMemory := memStat.UsedPercent

			if smoothedCPU == 0 {
				smoothedCPU = currentCPU
				smoothedMemory = currentMemory
			} else {
				smoothedCPU = 0.7*smoothedCPU + 0.3*currentCPU
				smoothedMemory = 0.7*smoothedMemory + 0.3*currentMemory
			}

			metrics.mu.Lock()
			metrics.CPU = math.Max(0, math.Min(100, smoothedCPU))
			metrics.Memory = math.Max(0, math.Min(100, smoothedMemory))
			metrics.mu.Unlock()
		}

		time.Sleep(2 * time.Second)
	}
}
