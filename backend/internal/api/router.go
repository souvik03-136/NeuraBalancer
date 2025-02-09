package api

import (
	"github.com/labstack/echo/v4"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

// RegisterRoutes sets up API endpoints
func RegisterRoutes(e *echo.Echo, lb *loadbalancer.LoadBalancer) {
	handler := &Handler{LB: lb} // Wrap LoadBalancer inside Handler

	e.GET("/", healthCheck)
	e.POST("/request", handler.HandleRequest) // Use handler's method
}

// Health check handler
func healthCheck(c echo.Context) error {
	return c.JSON(200, map[string]string{"status": "ok"})
}
