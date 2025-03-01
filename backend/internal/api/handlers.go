package api

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/souvik03-136/neurabalancer/backend/database"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

// Handler struct wraps LoadBalancer and Collector
type Handler struct {
	LB        *loadbalancer.LoadBalancer
	Collector *metrics.Collector
}

// Request struct
type Request struct {
	ClientID string `json:"client_id"`
}

// Response struct
type Response struct {
	Server string `json:"server"`
}

// HandleRequest forwards requests to all healthy servers (Broadcast)
// HandleRequest forwards requests to all healthy servers (Broadcast)
func (h *Handler) HandleRequest(c echo.Context) error {
	startTime := time.Now()

	// Read the request body once
	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Println("❌ Failed to read request body:", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read request body"})
	}
	c.Request().Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var wg sync.WaitGroup
	var mu sync.Mutex
	responses := make([]map[string]interface{}, 0)
	var failedServers []map[string]interface{}

	// Get all servers from load balancer
	servers := h.LB.GetAllServers()

	// Filter healthy servers
	healthyServers := make([]*loadbalancer.Server, 0)
	for _, server := range servers {
		if server.Alive {
			healthyServers = append(healthyServers, server)
		}
	}

	if len(healthyServers) == 0 {
		log.Println("❌ No healthy servers available")
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "No healthy servers available"})
	}

	// Broadcast to all healthy servers
	for _, server := range healthyServers {
		wg.Add(1)
		go func(s *loadbalancer.Server) {
			defer wg.Done()

			// Forward request
			resp, err := http.Post(s.URL+"/process", "application/json", bytes.NewReader(bodyBytes))
			success := err == nil
			responseTime := time.Since(startTime)

			// Record metrics with actual server ID
			h.Collector.RecordRequest(s.ID, success, responseTime)

			// Handle response
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				failedServers = append(failedServers, map[string]interface{}{
					"server": s.URL,
					"error":  err.Error(),
				})
				return
			}

			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			responses = append(responses, map[string]interface{}{
				"server": s.URL,
				"status": resp.StatusCode,
				"body":   string(body),
			})
		}(server)
	}

	wg.Wait()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"responses":      responses,
		"failed_servers": failedServers,
	})
}

// LoadBalancerHealthCheck provides the health status of backend servers
func (h *Handler) LoadBalancerHealthCheck(c echo.Context) error {
	healthyServers := []string{}
	unhealthyServers := []string{}
	var updateMutex sync.Mutex

	// Classify servers into healthy and unhealthy lists
	for _, server := range h.LB.GetAllServers() {
		if server.Alive {
			healthyServers = append(healthyServers, server.URL)
			// Update server as active in the database
			updateMutex.Lock()
			err := database.UpdateServerStatus(server.ID, true)
			updateMutex.Unlock()
			if err != nil {
				log.Printf("❌ Failed to update server status for %s: %v", server.URL, err)
			}
		} else {
			unhealthyServers = append(unhealthyServers, server.URL)
			// Update server as inactive in the database
			updateMutex.Lock()
			err := database.UpdateServerStatus(server.ID, false)
			updateMutex.Unlock()
			if err != nil {
				log.Printf("❌ Failed to update server status for %s: %v", server.URL, err)
			}
		}
	}

	// Determine response status
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

// GetMetrics exposes Prometheus metrics
func (h *Handler) GetMetrics(c echo.Context) error {
	promHandler := promhttp.Handler()
	promHandler.ServeHTTP(c.Response(), c.Request())
	return nil
}

// RegisterServer registers a new server in the database.
// RegisterServer registers a new server in the database.
func (h *Handler) RegisterServer(c echo.Context) error {
	ip := c.QueryParam("ip")
	if ip == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Missing IP address"})
	}

	// Inserting server into the database
	_, err := database.DB.Exec("INSERT INTO servers (ip, status) VALUES (?, ?)", ip, true)
	if err != nil {
		log.Printf("❌ Failed to register server: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to register server"})
	}

	log.Printf("✅ Server %s registered successfully", ip)
	return c.JSON(http.StatusOK, map[string]string{"message": "Server registered successfully"})
}

// GetLeastLoadedServer retrieves the server with the lowest load.
func (h *Handler) GetLeastLoadedServer(c echo.Context) error {
	ip, err := database.GetLeastLoadedServer()
	if err != nil {
		log.Printf("❌ Failed to fetch least loaded server: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch least loaded server"})
	}

	return c.JSON(http.StatusOK, map[string]string{"server_ip": ip})
}
