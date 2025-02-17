package api

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

// CORSMiddleware sets up CORS headers
func CORSMiddleware() echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS", "PUT", "DELETE"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
	})
}

// RequestLogger logs each incoming request
func RequestLogger() echo.MiddlewareFunc {
	return middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "🕒 ${time_rfc3339} ➡️ [${method}] ${uri} - ${remote_ip}\n⬅️  [${status}] ${uri}\n",
	})
}

// ✅ Improved MetricsMiddleware for safer handling of `serverID`
func MetricsMiddleware(collector *metrics.Collector) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			duration := time.Since(start)

			status := c.Response().Status
			success := status >= 200 && status < 400 // 2xx and 3xx are considered successful

			// Extract server ID from request headers
			serverIDStr := strings.TrimSpace(c.Request().Header.Get("X-Server-ID"))
			serverID := -1 // Default unknown server ID

			if serverIDStr != "" {
				if id, convErr := strconv.Atoi(serverIDStr); convErr == nil {
					serverID = id
				} else {
					log.Printf("⚠️ Invalid server ID received: %q, error: %v", serverIDStr, convErr)
				}
			}

			// Collect metrics
			collector.RecordRequest(serverID, success, duration)

			return err
		}
	}
}
