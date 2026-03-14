// File: backend/internal/api/router.go
package api

import (
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

// NewRouter creates and wires the Echo router with all middleware and routes.
func NewRouter(lb *loadbalancer.LoadBalancer, logger *zap.Logger, serviceName string) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	// ── Middleware stack (order matters) ──────────────────────────────────────
	e.Use(Recover(logger))
	e.Use(RequestID())
	e.Use(CORS())
	e.Use(StructuredLogger(logger))
	e.Use(otelecho.Middleware(serviceName))
	e.Use(RateLimiter())

	h := NewHandler(lb, logger)

	// ── Health / readiness probes (no auth required) ─────────────────────────
	e.GET("/health/live", h.HealthCheck)
	e.GET("/health/ready", h.ReadinessCheck)

	// ── Prometheus scrape endpoint ────────────────────────────────────────────
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// ── Versioned API ─────────────────────────────────────────────────────────
	v1 := e.Group("/api/v1")
	v1.GET("/servers", h.ServersStatus)
	v1.Any("/request", h.ProxyHandler)
	v1.Any("/request/*", h.ProxyHandler)

	return e
}
