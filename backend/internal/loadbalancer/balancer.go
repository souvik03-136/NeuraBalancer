// File: backend/internal/loadbalancer/balancer.go
package loadbalancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/config"
	"github.com/souvik03-136/neurabalancer/backend/internal/database"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
	"github.com/souvik03-136/neurabalancer/backend/internal/tracer"
)

// Server represents a single backend server.
type Server struct {
	ID          int
	URL         string
	Alive       bool
	Weight      int
	Capacity    int
	Connections int // active in-flight requests
	mu          sync.Mutex
}

// CanAcceptRequest returns true if the server is alive and has capacity headroom.
func (s *Server) CanAcceptRequest() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Alive && s.Connections < s.Capacity
}

func (s *Server) incrementConns() {
	s.mu.Lock()
	s.Connections++
	s.mu.Unlock()
}

func (s *Server) decrementConns() {
	s.mu.Lock()
	if s.Connections > 0 {
		s.Connections--
	}
	s.mu.Unlock()
}

// LoadBalancer routes requests to backend servers using a pluggable Strategy.
type LoadBalancer struct {
	servers  []*Server
	mu       sync.RWMutex
	strategy Strategy
	cfg      *config.HealthConfig
	db       *database.DB
	col      *metrics.Collector
	logger   *zap.Logger
	stop     chan struct{}
}

// New creates a LoadBalancer, seeds the server list from config, and starts
// background health checking and metrics polling.
func New(
	strategy Strategy,
	serverURLs []string,
	cfg *config.HealthConfig,
	db *database.DB,
	col *metrics.Collector,
	logger *zap.Logger,
) *LoadBalancer {
	lb := &LoadBalancer{
		strategy: strategy,
		cfg:      cfg,
		db:       db,
		col:      col,
		logger:   logger,
		stop:     make(chan struct{}),
	}

	ctx := context.Background()

	for _, rawURL := range serverURLs {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}

		// Upsert into DB so the server exists before we query it
		if err := db.UpsertServer(ctx, rawURL); err != nil {
			logger.Warn("failed to upsert server", zap.String("url", rawURL), zap.Error(err))
		}

		id, err := db.GetServerID(ctx, rawURL)
		if err != nil {
			logger.Warn("server ID lookup failed, using ephemeral ID",
				zap.String("url", rawURL), zap.Error(err))
			id = -(len(lb.servers) + 1)
		}

		capacity := 10
		if id > 0 {
			if cap, err := db.GetServerCapacity(ctx, id); err == nil {
				capacity = cap
			}
		}

		weight := 1
		if id > 0 {
			if w, err := db.GetServerWeight(ctx, id); err == nil {
				weight = w
			}
		}

		lb.servers = append(lb.servers, &Server{
			ID:       id,
			URL:      rawURL,
			Alive:    true, // optimistic; health check corrects quickly
			Weight:   weight,
			Capacity: capacity,
		})
	}

	go lb.runHealthChecks()
	go lb.runMetricsPolling()

	return lb
}

// Stop signals background goroutines to exit. Call on shutdown.
func (lb *LoadBalancer) Stop() {
	close(lb.stop)
}

// NextServer picks the best available server according to the active strategy.
func (lb *LoadBalancer) NextServer(ctx context.Context) (*Server, error) {
	lb.mu.RLock()
	healthy := make([]*Server, 0, len(lb.servers))
	for _, s := range lb.servers {
		if s.Alive {
			healthy = append(healthy, s)
		}
	}
	lb.mu.RUnlock()

	if len(healthy) == 0 {
		return nil, errors.New("no healthy servers available")
	}

	selected := lb.strategy.Select(healthy)
	if selected == nil {
		return nil, errors.New("strategy returned nil server")
	}

	selected.incrementConns()
	lb.col.SetActiveConnections(selected.ID, selected.Connections)
	return selected, nil
}

// ReleaseServer decrements the connection counter for a server.
func (lb *LoadBalancer) ReleaseServer(s *Server) {
	s.decrementConns()
	lb.col.SetActiveConnections(s.ID, s.Connections)
}

// Servers returns a snapshot of all servers (healthy and unhealthy).
func (lb *LoadBalancer) Servers() []*Server {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	snap := make([]*Server, len(lb.servers))
	copy(snap, lb.servers)
	return snap
}

// ProxyRequest forwards an incoming request to a selected backend server.
// It records duration, success, and tracing spans.
func (lb *LoadBalancer) ProxyRequest(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Tracer("loadbalancer").Start(r.Context(), "ProxyRequest")
	defer span.End()

	server, err := lb.NextServer(ctx)
	if err != nil {
		lb.logger.Warn("no server available", zap.Error(err))
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer lb.ReleaseServer(server)

	span.SetAttributes(
		attribute.String("lb.server.url", server.URL),
		attribute.Int("lb.server.id", server.ID),
	)

	start := time.Now()
	targetURL := server.URL + r.URL.RequestURI()
	proxyReq, err := http.NewRequestWithContext(ctx, r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	proxyReq.Header = r.Header.Clone()
	// Propagate trace context downstream

	resp, err := http.DefaultClient.Do(proxyReq)
	duration := time.Since(start)

	success := err == nil && resp != nil && resp.StatusCode < 500
	statusCode := http.StatusBadGateway
	if err == nil && resp != nil {
		statusCode = resp.StatusCode
	}

	lb.col.RecordRequest(ctx, server.ID, r.Method, r.URL.Path, statusCode, duration, success)

	if err != nil {
		lb.logger.Warn("backend request failed",
			zap.String("server", server.URL), zap.Error(err))
		lb.markServerDown(ctx, server)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers and body
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	buf := make([]byte, 32*1024)
	if _, copyErr := copyResponse(w, resp.Body, buf); copyErr != nil {
		lb.logger.Warn("response copy error", zap.Error(copyErr))
	}
}

// ─── background loops ─────────────────────────────────────────────────────────

func (lb *LoadBalancer) runHealthChecks() {
	interval := time.Duration(lb.cfg.IntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-lb.stop:
			return
		case <-ticker.C:
			lb.checkAllServers()
		}
	}
}

func (lb *LoadBalancer) checkAllServers() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(lb.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	for _, s := range lb.servers {
		wasAlive := s.Alive
		isAlive := lb.pingServer(ctx, s)

		if wasAlive != isAlive {
			s.Alive = isAlive
			if s.ID > 0 {
				if err := lb.db.UpdateServerStatus(ctx, s.ID, isAlive); err != nil {
					lb.logger.Warn("failed to update server status in DB",
						zap.Int("id", s.ID), zap.Error(err))
				}
			}
			lb.logger.Info("server health changed",
				zap.String("url", s.URL), zap.Bool("alive", isAlive))
			if !isAlive {
				s.Connections = 0 // reset stale counter
			}
		}
	}
}

func (lb *LoadBalancer) pingServer(ctx context.Context, s *Server) bool {
	client := &http.Client{Timeout: time.Duration(lb.cfg.TimeoutSeconds) * time.Second}
	url := s.URL + "/health"
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}
	for i := 0; i < lb.cfg.Retries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return true
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func (lb *LoadBalancer) runMetricsPolling() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-lb.stop:
			return
		case <-ticker.C:
			ctx := context.Background()
			lb.mu.RLock()
			servers := make([]*Server, len(lb.servers))
			copy(servers, lb.servers)
			lb.mu.RUnlock()

			for _, s := range servers {
				if s.Alive && s.ID > 0 {
					lb.col.UpdateServerMetrics(ctx, s.ID, s.URL)
				}
			}
		}
	}
}

func (lb *LoadBalancer) markServerDown(ctx context.Context, s *Server) {
	lb.mu.Lock()
	s.Alive = false
	lb.mu.Unlock()

	if s.ID > 0 {
		if err := lb.db.UpdateServerStatus(ctx, s.ID, false); err != nil {
			lb.logger.Warn("failed to mark server down", zap.Int("id", s.ID), zap.Error(err))
		}
	}
	lb.logger.Warn("server marked down", zap.String("url", s.URL))
}

// ─── response copy helper ─────────────────────────────────────────────────────

func copyResponse(dst http.ResponseWriter, src interface{ Read([]byte) (int, error) }, buf []byte) (int64, error) {
	var written int64
	for {
		nr, err := src.Read(buf)
		if nr > 0 {
			nw, writeErr := dst.Write(buf[:nr])
			written += int64(nw)
			if writeErr != nil {
				return written, writeErr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				return written, nil
			}
			return written, fmt.Errorf("read error: %w", err)
		}
	}
}
