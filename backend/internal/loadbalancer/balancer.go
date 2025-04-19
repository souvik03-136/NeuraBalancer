package loadbalancer

import (
	"bytes"
	"errors"
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
	ID          int
	URL         string
	Alive       bool
	Weight      int
	Connections int // Track active connections
	Capacity    int // Add capacity field
	mu          sync.Mutex
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

	for _, serverURL := range serverURLs {
		serverID, err := database.GetServerID(serverURL)
		if err != nil {
			log.Printf("Using temporary ID for %s: %v", serverURL, err)
			serverID = -len(lb.servers) // Generate unique negative ID
		}

		capacity := 1 // Default capacity

		if serverID > 0 {
			capacity, err = database.GetServerCapacity(serverID)
			if err != nil {
				log.Printf("Using default capacity for %s: %v", serverURL, err)
			}
		}

		weight := 1 // Default weight
		if serverID > 0 {
			weight, err = database.GetServerWeight(serverID)
			if err != nil {
				log.Printf("Using default weight for %s: %v", serverURL, err)
			}
		}
		server := &Server{
			ID:       serverID,
			URL:      serverURL,
			Weight:   weight,
			Capacity: capacity,
		}
		isActive, err := database.GetServerActiveStatus(serverID)
		if err != nil {
			log.Printf("Failed to get initial status for %s: %v", serverURL, err)
			isActive = true // Default to active
		}
		server.Alive = isActive
		lb.servers = append(lb.servers, server)
	}

	go lb.startHealthChecks()
	return lb
}

// GetServer selects a healthy server based on the strategy
func (lb *LoadBalancer) GetServer() (*Server, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Filter healthy servers
	var healthyServers []*Server
	for _, s := range lb.servers {
		if s.Alive {
			healthyServers = append(healthyServers, s)
		}
	}

	if len(healthyServers) == 0 {
		return nil, errors.New("no healthy servers available")
	}

	selected := lb.strategy.SelectServer(healthyServers)
	if selected == nil {
		return nil, errors.New("strategy failed to select server")
	}

	// Increment connection count
	selected.mu.Lock()
	selected.Connections++
	selected.mu.Unlock()

	return selected, nil
}

// checkServerHealth pings the server's /health endpoint
const healthCheckRetries = 3

func (s *Server) checkServerHealth() bool {
	client := http.Client{Timeout: 3 * time.Second}

	// Ensure URL has scheme
	healthURL := s.URL + "/health"
	if !strings.HasPrefix(healthURL, "http") {
		healthURL = "http://" + healthURL
	}

	for i := 0; i < healthCheckRetries; i++ {
		resp, err := client.Get(healthURL)
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				log.Printf("Server %s is UP\n", s.URL)
				return true
			}
			log.Printf("Server %s returned %d (unhealthy)\n", s.URL, resp.StatusCode)
		} else {
			log.Printf("Server %s is DOWN: %v\n", s.URL, err)
		}

		// Retry after a short delay
		if i < healthCheckRetries-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return false
}

// startHealthChecks runs periodic health checks
func (lb *LoadBalancer) startHealthChecks() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		lb.mu.Lock()
		for _, s := range lb.servers {
			wasAlive := s.Alive
			isAlive := s.checkServerHealth()

			// Only update DB when status changes
			if wasAlive != isAlive {
				err := database.UpdateServerStatus(s.ID, isAlive)
				if err != nil {
					log.Printf("Failed to update server status (ID %d): %v", s.ID, err)
				} else {
					log.Printf("Updated server %s status in DB: Alive=%v", s.URL, isAlive)
				}
			}

			s.Alive = isAlive
			if !wasAlive && isAlive {
				s.mu.Lock()
				s.Connections = 0
				s.mu.Unlock()
			}
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
	startTime := time.Now()
	var serverID int
	var success bool

	defer func() {
		duration := time.Duration(time.Since(startTime).Milliseconds()) * time.Millisecond
		lb.metrics.RecordRequest(serverID, success, duration)
	}()

	// Select server
	server, err := lb.GetServer()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		success = false
		return
	}
	serverID = server.ID

	// Decrement connection count when done
	defer func() {
		server.mu.Lock()
		server.Connections--
		server.mu.Unlock()
	}()

	// Clone request
	bodyBytes, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	// Create new request
	req, err := http.NewRequest(r.Method, server.URL+r.URL.Path, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, "Error creating request", http.StatusInternalServerError)
		success = false
		return
	}
	req.Header = r.Header.Clone()

	// Record initial attempt before processing
	if err := database.InsertAttempt(r.Context(), server.ID, false); err != nil {
		log.Printf("Failed to log initial attempt: %v", err)
	}

	// Forward request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Record failed attempt
		if err := database.InsertAttempt(r.Context(), server.ID, false); err != nil {
			log.Printf("Failed to log failed attempt: %v", err)
		}

		// Mark server as down immediately
		server.mu.Lock()
		server.Alive = false
		server.mu.Unlock()

		// Async DB update
		go func() {
			if err := database.UpdateServerStatus(server.ID, false); err != nil {
				log.Printf("Failed to update status for %s: %v", server.URL, err)
			}
		}()

		success = false
		http.Error(w, "Server unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Record successful attempt
	if err := database.InsertAttempt(r.Context(), server.ID, true); err != nil {
		log.Printf("Failed to log successful attempt: %v", err)
	}

	// Copy response
	success = resp.StatusCode < 500
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// SelectBackend dynamically chooses the best backend server.
func SelectBackend() string {
	serverIP, err := database.GetLeastLoadedServer()
	if err != nil {
		log.Println("Error fetching backend server:", err)
		return "" // Fallback strategy (e.g., round robin)
	}
	log.Println("Routing request to:", serverIP)
	return serverIP
}
