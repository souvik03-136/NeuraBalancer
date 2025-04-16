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

	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())

	// Load server list from ENV or fallback
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
		err := database.RegisterServer(server)
		if err != nil {
			log.Printf("Failed to register server %s in DB: %v", server, err)
		} else {
			log.Printf("Server %s registered in DB", server)
		}
	}

	// Wait for servers to start
	time.Sleep(2 * time.Second)

	log.Println("Available Servers:", serverList)

	// Check server health
	for _, server := range serverList {
		if isServerUp(server) {
			log.Printf("Server %s is UP!", server)
		} else {
			log.Printf("Server %s is DOWN!", server)
		}
	}

	// Load ML model endpoint
	mlEndpoint := os.Getenv("ML_MODEL_ENDPOINT")
	if mlEndpoint == "" {
		mlEndpoint = "http://ml-service:8000"
	}

	// Initialize Metrics Collector
	collector := metrics.NewCollector()

	// Define load balancing strategies
	roundRobin := &loadbalancer.RoundRobinStrategy{}
	leastConnections := &loadbalancer.LeastConnectionsStrategy{}
	weightedRoundRobin := &loadbalancer.WeightedRoundRobinStrategy{}
	randomSelection := &loadbalancer.RandomSelectionStrategy{}
	mlStrategy := loadbalancer.NewMLStrategy(mlEndpoint, collector)

	// Select strategy from ENV
	var strategy loadbalancer.Strategy
	strategyEnv := strings.ToLower(os.Getenv("LB_STRATEGY"))

	switch strategyEnv {
	case "ml":
		strategy = mlStrategy
		log.Println("Using AI-Driven ML Strategy")
	case "least_connections":
		strategy = leastConnections
	case "weighted_round_robin":
		strategy = weightedRoundRobin
	case "random":
		strategy = randomSelection
	case "round_robin":
		strategy = roundRobin
	default:
		log.Println("No valid strategy found. Defaulting to Least Connections.")
		strategyEnv = "least_connections"
		strategy = leastConnections
	}

	log.Println("Load Balancing Strategy:", strategyEnv)

	// Echo setup
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Load Balancer with fallback server list
	lb := loadbalancer.NewLoadBalancer(strategy, fallbackServerList(serverList))

	// Register routes
	api.RegisterRoutes(e, lb, collector)

	// Prometheus metrics endpoint
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// Load port from ENV or fallback
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server
	go func() {
		log.Println("Starting Load Balancer on port", port)
		if err := e.Start(":" + port); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server gracefully...")

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
	if len(servers) == 0 {
		log.Println("No active servers in DB, falling back to original list")
		return serverList
	}
	return servers
}
