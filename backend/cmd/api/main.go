package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/souvik03-136/neurabalancer/backend/database"
	"github.com/souvik03-136/neurabalancer/backend/internal/api"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

func main() {
	// Initialize Database
	if err := database.InitDB(); err != nil {
		log.Fatalf("❌ Database initialization failed: %v", err)
	}
	defer database.CloseDB()

	// Seed the random number generator for Random Selection strategy
	rand.Seed(time.Now().UnixNano())

	// Define server list (from ENV or fallback to defaults)
	serverListEnv := os.Getenv("SERVERS")
	var serverList []string

	if serverListEnv == "" {
		log.Println("⚠️  No SERVERS environment variable found. Using default servers.")
		serverList = []string{"http://localhost:5000", "http://localhost:5001", "http://localhost:5002"}
	} else {
		serverList = strings.Split(serverListEnv, ",")
	}

	// Register servers in database
	for _, server := range serverList {
		err := database.RegisterServer(server) // Auto-register in DB
		if err != nil {
			log.Printf("⚠️ Failed to register server %s in DB: %v", server, err)
		} else {
			log.Printf("✅ Server %s registered in DB", server)
		}
	}

	// Start backend servers in goroutines
	for _, server := range serverList {
		go startBackendServer(server)
	}

	// Wait a bit for servers to start
	time.Sleep(2 * time.Second)

	log.Println("🔗 Available Servers:", serverList)

	// Check if servers are reachable
	for _, server := range serverList {
		if isServerUp(server) {
			log.Printf("✅ Server %s is UP!", server)
		} else {
			log.Printf("❌ Server %s is DOWN!", server)
		}
	}

	// Define available load balancing strategies
	roundRobin := &loadbalancer.RoundRobinStrategy{}
	leastConnections := &loadbalancer.LeastConnectionsStrategy{}
	weightedRoundRobin := &loadbalancer.WeightedRoundRobinStrategy{}
	randomSelection := &loadbalancer.RandomSelectionStrategy{}

	// Select strategy from ENV (default: Round Robin)
	var strategy loadbalancer.Strategy
	strategyEnv := strings.ToLower(os.Getenv("LB_STRATEGY"))

	switch strategyEnv {
	case "least_connections":
		strategy = leastConnections
	case "weighted_round_robin":
		strategy = weightedRoundRobin
	case "random":
		strategy = randomSelection
	default:
		log.Println("⚠️  No valid strategy found. Defaulting to Round Robin.")
		strategyEnv = "round_robin"
		strategy = roundRobin
	}

	log.Println("🔄 Load Balancing Strategy:", strategyEnv)

	// Initialize Echo framework
	e := echo.New()
	e.Use(middleware.Logger())  // Request Logging
	e.Use(middleware.Recover()) // Panic Recovery
	e.Use(middleware.CORS())    // CORS Middleware

	// Initialize Metrics (singleton)
	collector := metrics.NewCollector()

	// Initialize Load Balancer with fallback
	lb := loadbalancer.NewLoadBalancer(strategy, fallbackServerList(serverList))

	// Register API routes with metrics integration
	api.RegisterRoutes(e, lb, collector)

	// Expose Prometheus metrics endpoint
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Define port (from ENV or fallback to 8080)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start Load Balancer in a goroutine
	go func() {
		log.Println("🚀 Starting Load Balancer on port", port)
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ Error starting server: %v", err)
		}
	}()

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("🛑 Shutting down server gracefully...")

	// Context for graceful shutdown (timeout: 10 seconds)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server exited cleanly")
}

// startBackendServer runs a simple backend server
func startBackendServer(serverAddr string) {
	parsed, err := url.Parse(serverAddr)
	if err != nil {
		log.Fatalf("❌ Invalid server URL: %s, error: %v", serverAddr, err)
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

	// Add other endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from server on port %s", port)
	})

	log.Printf("🚀 Backend server starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("❌ Backend server failed on port %s: %v", port, err)
	}
}

// isServerUp checks if a server is reachable with retries
func isServerUp(server string) bool {
	client := http.Client{Timeout: 5 * time.Second}
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		resp, err := client.Get(fmt.Sprintf("%s/health", server))
		if err == nil && resp.StatusCode == http.StatusOK {
			return true
		}
		log.Printf("🔍 Retrying connection to %s (attempt %d/%d)", server, i+1, maxRetries)
		time.Sleep(2 * time.Second)
	}
	return false
}

// fallbackServerList provides a fallback if DB queries fail
func fallbackServerList(serverList []string) []string {
	servers, err := database.GetAvailableServers()
	if err != nil || len(servers) == 0 {
		log.Println("⚠️ No servers in DB, falling back to in-memory list.")
		return serverList
	}
	return servers
}
