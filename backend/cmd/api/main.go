package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/souvik03-136/neurabalancer/backend/database"
	"github.com/souvik03-136/neurabalancer/backend/internal/api"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

func main() {
	// Initialize Database
	if err := database.InitDB(); err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer database.CloseDB()

	// Seed the random number generator for Random Selection strategy
	rand.Seed(time.Now().UnixNano())

	// Define server list (from ENV or fallback to defaults)
	serverListEnv := os.Getenv("SERVERS")
	var serverList []string

	if serverListEnv == "" {
		log.Println("No SERVERS environment variable found. Using default servers.")
		serverList = []string{"http://localhost:5000", "http://localhost:5001", "http://localhost:5002"}
	} else {
		serverList = strings.Split(serverListEnv, ",")
	}

	// Register servers in database
	for _, server := range serverList {
		err := database.RegisterServer(server) // Auto-register in DB
		if err != nil {
			log.Printf("Failed to register server %s in DB: %v", server, err)
		} else {
			log.Printf("Server %s registered in DB", server)
		}
	}

	// Wait a bit for servers to start
	time.Sleep(2 * time.Second)

	log.Println("Available Servers:", serverList)

	// Check if servers are reachable
	for _, server := range serverList {
		if isServerUp(server) {
			log.Printf("Server %s is UP!", server)
		} else {
			log.Printf("Server %s is DOWN!", server)
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
	case "round_robin":
		strategy = roundRobin
	default: // Default to Least Connections
		log.Println("No valid strategy found. Defaulting to Least Connections.")
		strategyEnv = "least_connections"
		strategy = leastConnections
	}

	log.Println("Load Balancing Strategy:", strategyEnv)

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
		log.Println("Starting Load Balancer on port", port)
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server gracefully...")

	// Context for graceful shutdown (timeout: 10 seconds)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := e.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly")
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
		log.Printf("Retrying connection to %s (attempt %d/%d)", server, i+1, maxRetries)
		time.Sleep(2 * time.Second)
	}
	return false
}

// fallbackServerList provides a fallback if DB queries fail
func fallbackServerList(serverList []string) []string {
	servers, err := database.GetAvailableServers()
	if err != nil {
		log.Printf("DB error, using in-memory list: %v", err)
		return serverList
	}

	// Directly use the server strings from DB
	if len(servers) == 0 {
		log.Println("No active servers in DB, falling back to original list")
		return serverList
	}
	return servers
}
