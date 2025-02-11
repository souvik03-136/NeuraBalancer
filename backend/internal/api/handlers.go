package api

import (
	"log"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

// Handler struct that wraps LoadBalancer
type Handler struct {
	LB *loadbalancer.LoadBalancer
}

// Request struct
type Request struct {
	ClientID string `json:"client_id"`
}

// Response struct
type Response struct {
	Server string `json:"server"`
}

// HandleRequest - Forwards request to all healthy servers (Broadcast)
func (h *Handler) HandleRequest(c echo.Context) error {
	var wg sync.WaitGroup
	responses := make([]map[string]interface{}, 0)
	mu := sync.Mutex{}

	// Get all healthy servers
	servers := h.LB.GetAllServers()
	healthyServers := []*loadbalancer.Server{}
	for _, server := range servers {
		if server.Alive {
			healthyServers = append(healthyServers, server)
		}
	}

	// If no healthy servers are found, return error
	if len(healthyServers) == 0 {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "No healthy servers available"})
	}

	// Broadcast request to all healthy servers
	for _, server := range healthyServers {
		wg.Add(1)
		go func(serverURL string) {
			defer wg.Done()

			// Forward the request
			resp, err := http.Post(serverURL+"/process", "application/json", c.Request().Body)
			if err != nil {
				log.Printf("❌ Failed to forward request to %s: %v", serverURL, err)
				return
			}
			defer resp.Body.Close()

			// Collect responses
			responseData := map[string]interface{}{
				"server": serverURL,
				"status": resp.StatusCode,
			}
			mu.Lock()
			responses = append(responses, responseData)
			mu.Unlock()
		}(server.URL)
	}

	wg.Wait() // Wait for all requests to complete

	return c.JSON(http.StatusOK, responses)
}

// App health check endpoint (for load balancer itself)
func healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "load balancer running"})
}

// Load balancer health check (checks backend servers)
func (h *Handler) LoadBalancerHealthCheck(c echo.Context) error {
	healthyServers := []string{}
	unhealthyServers := []string{}

	// Get a list of healthy/unhealthy servers
	for _, server := range h.LB.GetAllServers() {
		if server.Alive {
			healthyServers = append(healthyServers, server.URL)
		} else {
			unhealthyServers = append(unhealthyServers, server.URL)
		}
	}

	// Always return 200 for LB health, but mention if no backend servers are available
	statusCode := http.StatusOK
	if len(healthyServers) == 0 {
		statusCode = http.StatusServiceUnavailable
	}

	return c.JSON(statusCode, map[string]interface{}{
		"status":            "load balancer running",
		"healthy_servers":   healthyServers,
		"unhealthy_servers": unhealthyServers,
	})
}
