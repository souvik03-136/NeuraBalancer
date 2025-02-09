package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/souvik03-136/neurabalancer/backend/internal/api"
	"github.com/stretchr/testify/assert"
)

// Mock Load Balancer for testing
type MockLoadBalancer struct{}

func (m *MockLoadBalancer) SelectServer(_ []string) string {
	return "http://mockserver"
}

func TestAPIRequestHandling(t *testing.T) {
	e := echo.New()
	mockLB := &MockLoadBalancer{}

	api.RegisterRoutes(e, mockLB) // Register routes

	// Create a test request payload
	reqBody, _ := json.Marshal(map[string]string{"client_id": "test-client"})

	// Create a request
	req := httptest.NewRequest(http.MethodPost, "/request", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// Serve the request
	e.ServeHTTP(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"server":"http://mockserver"`)
}
