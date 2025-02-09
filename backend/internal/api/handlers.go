package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

// Handler struct that wraps LoadBalancer
type Handler struct {
	LB *loadbalancer.LoadBalancer
}

// Request struct
type Request struct {
	ClientID string `json:"client_id"`
}

// Response struct
type Response struct {
	Server string `json:"server"`
}

// HandleRequest processes incoming requests through the LoadBalancer
func (h *Handler) HandleRequest(c echo.Context) error {
	req := new(Request)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}

	server, err := h.LB.GetServer()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "no servers available"})
	}

	return c.JSON(http.StatusOK, Response{Server: server})
}
