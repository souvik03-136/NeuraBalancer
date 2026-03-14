// File: backend/internal/api/handlers_test.go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/api"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

func newTestHandler() (*api.Handler, *echo.Echo) {
	logger := zap.NewNop()
	lb := &loadbalancer.LoadBalancer{}
	h := api.NewHandler(lb, logger)
	e := echo.New()
	return h, e
}

func TestHealthCheck(t *testing.T) {
	h, e := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.HealthCheck(c); err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status field = %v, want ok", body["status"])
	}
}

func TestReadinessCheck_NoServers(t *testing.T) {
	logger := zap.NewNop()
	// A zero-value LoadBalancer has no servers → should return 503
	lb := &loadbalancer.LoadBalancer{}
	h := api.NewHandler(lb, logger)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.ReadinessCheck(c); err != nil {
		t.Fatalf("ReadinessCheck error: %v", err)
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
