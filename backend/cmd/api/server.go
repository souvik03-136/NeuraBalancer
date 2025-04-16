package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// startBackendServer runs a simple backend server
func startBackendServer(serverAddr string) {
	parsed, err := url.Parse(serverAddr)
	if err != nil {
		log.Fatalf("Invalid server URL: %s, error: %v", serverAddr, err)
	}

	port := parsed.Port()
	if port == "" {
		port = "80" // Default to port 80 if no port is specified
	}

	// Create the mux router first
	mux := http.NewServeMux()

	// Add metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Get real-time memory stats
		memStat, err := mem.VirtualMemory()
		if err != nil {
			log.Printf("Memory stats error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Get CPU usage with proper sampling
		cpuPercents, err := cpu.Percent(500*time.Millisecond, false)
		if err != nil || len(cpuPercents) == 0 {
			log.Printf("CPU measurement failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]float64{
			"cpu_usage":    cpuPercents[0],
			"memory_usage": memStat.UsedPercent, // Actual usage percentage
		})
	})

	// Add other endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from server on port %s", port)
	})

	log.Printf("Backend server starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Printf("Backend server failed on port %s: %v", port, err)
	}
}
