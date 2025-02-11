package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

// RegisterRoutes sets up API endpoints
func RegisterRoutes(e *echo.Echo, lb *loadbalancer.LoadBalancer) {
	handler := &Handler{LB: lb}

	// Middleware
	e.Use(middleware.Logger())  // Logs each request
	e.Use(middleware.Recover()) // Recovers from panics
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
	}))

	// Routes
	e.GET("/", healthCheck)                           // Health check for API
	e.GET("/health", handler.LoadBalancerHealthCheck) // Health check for LB
	e.POST("/request", handler.HandleRequest)         // Broadcast request to all servers
}

// HealthCheck returns a basic status response
func (h *Handler) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "load balancer running"})
}
