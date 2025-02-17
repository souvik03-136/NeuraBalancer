package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

// ✅ Move HealthCheck above RegisterRoutes for better readability
func (h *Handler) HealthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "load balancer running"})
}

// RegisterRoutes sets up API endpoints
func RegisterRoutes(e *echo.Echo, lb *loadbalancer.LoadBalancer, collector *metrics.Collector) {
	handler := &Handler{LB: lb, Collector: collector}

	// Middleware
	e.Use(middleware.Logger())  // Logs requests
	e.Use(middleware.Recover()) // Panic recovery
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
	}))

	// ✅ Apply Metrics Middleware
	e.Use(MetricsMiddleware(collector))

	// Routes
	e.GET("/", handler.HealthCheck)                   // ✅ API health check
	e.GET("/health", handler.LoadBalancerHealthCheck) // ✅ Load Balancer health check
	e.POST("/request", handler.HandleRequest)         // ✅ Request forwarding (includes server_id handling)
	e.GET("/metrics", handler.GetMetrics)             // ✅ Prometheus Metrics Endpoint
}
