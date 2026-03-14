// File: backend/internal/api/handlers.go
package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

// Handler holds all dependencies for HTTP handlers.
type Handler struct {
	lb     *loadbalancer.LoadBalancer
	logger *zap.Logger
}

// NewHandler constructs a Handler.
func NewHandler(lb *loadbalancer.LoadBalancer, logger *zap.Logger) *Handler {
	return &Handler{lb: lb, logger: logger}
}

// HealthCheck responds with a simple liveness indicator.
// GET /health/live
func (h *Handler) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "ts": time.Now().UTC()})
}

// ReadinessCheck verifies that at least one backend server is healthy.
// GET /health/ready
func (h *Handler) ReadinessCheck(c echo.Context) error {
	servers := h.lb.Servers()
	healthy := 0
	for _, s := range servers {
		if s.Alive {
			healthy++
		}
	}
	if healthy == 0 {
		return c.JSON(http.StatusServiceUnavailable, echo.Map{
			"status":  "not ready",
			"reason":  "no healthy backend servers",
			"total":   len(servers),
			"healthy": 0,
		})
	}
	return c.JSON(http.StatusOK, echo.Map{
		"status":  "ready",
		"total":   len(servers),
		"healthy": healthy,
	})
}

// ServersStatus lists all backend servers and their current state.
// GET /api/v1/servers
func (h *Handler) ServersStatus(c echo.Context) error {
	servers := h.lb.Servers()
	type serverInfo struct {
		ID          int    `json:"id"`
		URL         string `json:"url"`
		Alive       bool   `json:"alive"`
		Weight      int    `json:"weight"`
		Capacity    int    `json:"capacity"`
		Connections int    `json:"active_connections"`
	}
	result := make([]serverInfo, len(servers))
	for i, s := range servers {
		result[i] = serverInfo{
			ID:          s.ID,
			URL:         s.URL,
			Alive:       s.Alive,
			Weight:      s.Weight,
			Capacity:    s.Capacity,
			Connections: s.Connections,
		}
	}
	return c.JSON(http.StatusOK, result)
}

// ProxyHandler routes an incoming request to a backend via the load balancer.
// POST /api/v1/request  (and catch-all proxy routes)
func (h *Handler) ProxyHandler(c echo.Context) error {
	h.lb.ProxyRequest(c.Response().Writer, c.Request())
	return nil
}
