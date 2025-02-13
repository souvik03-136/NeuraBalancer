package api

import (
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
		Format: "➡️  [${method}] ${uri} - ${remote_ip}\n⬅️  [${status}] ${uri}\n",
	})
}

// ✅ MetricsMiddleware is now properly used in `RegisterRoutes`
func MetricsMiddleware(collector *metrics.Collector) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			duration := time.Since(start)

			status := c.Response().Status
			success := status >= 200 && status < 400 // 2xx and 3xx are successful

			// Collect metrics
			collector.RecordRequest(success, duration)

			return err
		}
	}
}
