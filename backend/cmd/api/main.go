package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/souvik03-136/neurabalancer/backend/internal/api"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

func main() {
	// Initialize Echo framework
	e := echo.New()

	// Seed the random number generator for Random Selection strategy
	rand.Seed(time.Now().UnixNano())

	// Read server list from ENV variable (or default to hardcoded servers)
	serverListEnv := os.Getenv("SERVERS")
	var serverList []string

	if serverListEnv == "" {
		log.Println("⚠️  No SERVERS environment variable found. Using default servers.")
		serverList = []string{"http://server1", "http://server2", "http://server3"}
	} else {
		serverList = strings.Split(serverListEnv, ",")
	}

	log.Println("🔗 Available Servers:", serverList)

	// Load balancing strategies
	roundRobin := &loadbalancer.RoundRobinStrategy{}
	leastConnections := &loadbalancer.LeastConnectionsStrategy{}
	weightedRoundRobin := &loadbalancer.WeightedRoundRobinStrategy{}
	randomSelection := &loadbalancer.RandomSelectionStrategy{}

	// Select strategy from ENV
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
		strategy = roundRobin
	}

	log.Println("🔄 Load Balancing Strategy:", strategyEnv)

	// Create Load Balancer instance
	lb := loadbalancer.NewLoadBalancer(strategy, serverList)

	// Register API routes
	api.RegisterRoutes(e, lb)

	// Define port
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server in a goroutine
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
