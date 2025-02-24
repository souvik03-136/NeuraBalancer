package loadbalancer

import (
	"bytes"
	"context"
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
		metrics:  metrics.NewCollector(),
	}

	// Fetch server IDs from the database
	for _, serverURL := range serverURLs {
		serverID, err := database.GetServerID(serverURL)
		if err != nil {
			log.Printf("⚠️ Failed to get ID for server %s: %v", serverURL, err)
			continue
		}

		server := &Server{
			ID:     serverID, // Use the actual database ID
			URL:    serverURL,
			Alive:  true,
			Weight: 1,
		}
		lb.servers = append(lb.servers, server)
	}

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

	// Get healthy servers with proper locking
	lb.mu.Lock()
	healthyServers := make([]*Server, 0, len(lb.servers))
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

	// Clone request body and headers
	bodyBytes, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	wg.Add(len(healthyServers))

	for _, server := range healthyServers {
		go func(s *Server) {
			defer wg.Done()

			// Create new request with proper context
			ctx := context.WithValue(r.Context(), "lb_server_id", s.ID)
			req, _ := http.NewRequestWithContext(ctx, r.Method, s.URL+r.URL.Path, bytes.NewReader(bodyBytes))
			req.Header = r.Header.Clone()

			startTime := time.Now()
			resp, err := http.DefaultClient.Do(req)
			responseTime := time.Since(startTime)

			if err != nil {
				log.Printf("❌ Failed to forward to %s: %v", s.URL, err)
				lb.metrics.RecordRequest(s.ID, false, responseTime)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			mu.Lock()
			responses = append(responses, fmt.Sprintf("✅ %s: %s", s.URL, string(body)))
			mu.Unlock()

			lb.metrics.RecordRequest(s.ID, true, responseTime)
			log.Printf("✅ Successfully forwarded to %s", s.URL)
		}(server)
	}

	wg.Wait()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Broadcast complete to %d servers:\n%s",
		len(healthyServers),
		strings.Join(responses, "\n"))))
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
