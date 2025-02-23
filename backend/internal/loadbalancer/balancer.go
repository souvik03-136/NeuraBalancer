package loadbalancer

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/souvik03-136/neurabalancer/backend/database"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

// Server struct to track health status
type Server struct {
	ID     int
	URL    string
	Alive  bool
	Weight int // Added for Weighted Round Robin
}

// LoadBalancer struct
type LoadBalancer struct {
	servers  []*Server
	mu       sync.Mutex
	strategy Strategy
	metrics  *metrics.Collector // Add a metrics collector
}

// NewLoadBalancer initializes a load balancer with a given strategy
func NewLoadBalancer(strategy Strategy, serverURLs []string) *LoadBalancer {
	lb := &LoadBalancer{
		strategy: strategy,
		metrics:  metrics.NewCollector(), // Initialize the metrics collector
	}

	// Initialize servers with default weight (1)
	for _, url := range serverURLs {
		lb.servers = append(lb.servers, &Server{URL: url, Alive: true, Weight: 1})
	}

	// Start periodic health checks
	go lb.startHealthChecks()

	return lb
}

// GetServer selects a healthy server based on the strategy
func (lb *LoadBalancer) GetServer() (string, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Filter healthy servers
	var healthyServers []string
	for _, server := range lb.servers {
		if server.Alive {
			healthyServers = append(healthyServers, server.URL)
		}
	}

	if len(healthyServers) == 0 {
		log.Println("❌ No healthy servers available!")
		return "", errors.New("no healthy servers available")
	}

	selected := lb.strategy.SelectServer(healthyServers)
	log.Printf("🔀 Redirecting request to: %s\n", selected)
	return selected, nil
}

// checkServerHealth pings the server's /health endpoint
func (s *Server) checkServerHealth() bool {
	client := http.Client{Timeout: 5 * time.Second}

	// Ensure URL has scheme
	healthURL := s.URL + "/health"
	if !strings.HasPrefix(healthURL, "http") {
		healthURL = "http://" + healthURL
	}

	resp, err := client.Get(healthURL)
	if err != nil {
		log.Printf("❌ Server %s is DOWN: %v\n", s.URL, err)
		return false
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode == http.StatusOK
	if healthy {
		log.Printf("✅ Server %s is UP\n", s.URL)
	} else {
		log.Printf("⚠️  Server %s returned %d (unhealthy)\n", s.URL, resp.StatusCode)
	}
	return healthy
}

// startHealthChecks runs periodic health checks
func (lb *LoadBalancer) startHealthChecks() {
	// Perform an initial check
	lb.mu.Lock()
	for _, server := range lb.servers {
		server.Alive = server.checkServerHealth()
	}
	lb.mu.Unlock()

	// Start periodic checks
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		lb.mu.Lock()
		for _, server := range lb.servers {
			server.Alive = server.checkServerHealth()
		}
		lb.mu.Unlock()
	}
}

// GetAllServers returns a list of all servers with their health status
func (lb *LoadBalancer) GetAllServers() []*Server {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	serversCopy := make([]*Server, len(lb.servers))
	copy(serversCopy, lb.servers)
	return serversCopy
}

// ForwardRequest sends the request to ALL healthy backend servers (broadcast)
func (lb *LoadBalancer) ForwardRequest(w http.ResponseWriter, r *http.Request) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var responses []string

	lb.mu.Lock()
	healthyServers := []*Server{}
	for _, server := range lb.servers {
		if server.Alive {
			healthyServers = append(healthyServers, server)
		}
	}
	lb.mu.Unlock()

	if len(healthyServers) == 0 {
		log.Println("❌ No healthy servers available!")
		http.Error(w, "No healthy servers available", http.StatusServiceUnavailable)
		return
	}

	// Read the request body once and store it in memory
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	wg.Add(len(healthyServers))

	for _, server := range healthyServers {
		go func(server *Server) {
			defer wg.Done()

			// Create a new request with a cloned body
			bodyReader := bytes.NewReader(bodyBytes)
			startTime := time.Now()
			resp, err := http.Post(server.URL+r.URL.Path, r.Header.Get("Content-Type"), bodyReader)
			responseTime := time.Since(startTime)

			if err != nil {
				log.Printf("❌ Failed to forward request to %s: %v", server.URL, err)
				// Record failed request
				lb.metrics.RecordRequest(server.ID, false, responseTime)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			mu.Lock()
			responses = append(responses, fmt.Sprintf("✅ %s: %s", server.URL, string(body)))
			mu.Unlock()

			// Record successful request
			lb.metrics.RecordRequest(server.ID, true, responseTime)

			log.Printf("✅ Request forwarded to %s", server.URL)
		}(server)
	}

	// Wait for all requests to complete
	wg.Wait()

	// Respond to client
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Broadcast complete to all servers\n" + fmt.Sprintf("%v", responses)))
}

// SelectBackend dynamically chooses the best backend server.
func SelectBackend() string {
	serverIP, err := database.GetLeastLoadedServer()
	if err != nil {
		log.Println("⚠️ Error fetching backend server:", err)
		return "" // Fallback strategy (e.g., round robin)
	}
	log.Println("🔀 Routing request to:", serverIP)
	return serverIP
}
