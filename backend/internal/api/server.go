package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// StartBackendServer runs a simple backend server with graceful shutdown support
func StartBackendServer(serverAddr string) {
	parsed, err := url.Parse(serverAddr)
	if err != nil {
		log.Fatalf("❌ Invalid server URL: %s, error: %v", serverAddr, err)
	}

	port := parsed.Port()
	if port == "" {
		port = "80" // Default to port 80 if no port is specified
	}

	// Create the mux router
	mux := http.NewServeMux()

	// Add metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Get real-time memory stats
		memStat, err := mem.VirtualMemory()
		if err != nil {
			log.Printf("❌ Memory stats error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Get CPU usage with proper sampling
		cpuPercents, err := cpu.Percent(500*time.Millisecond, false)
		if err != nil || len(cpuPercents) == 0 {
			log.Printf("⚠️ CPU measurement failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]float64{
			"cpu_usage":    cpuPercents[0],
			"memory_usage": memStat.UsedPercent, // Actual usage percentage
		})
	})

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	// Add root endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from server on port %s", port)
	})

	// Create HTTP server with timeouts
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	// Channel to listen for shutdown signals
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		log.Printf("🚀 Backend server starting on port %s...\n", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Backend server failed on port %s: %v", port, err)
		}
	}()

	// Block until shutdown signal is received
	<-shutdownChan
	log.Printf("🛑 Shutting down server on port %s...\n", port)

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Server on port %s forced to shutdown: %v", port, err)
	}

	log.Printf("✅ Server on port %s exited cleanly\n", port)
}
