// File: backend/internal/loadbalancer/ml_strategy.go
package loadbalancer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/config"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

// MLStrategy selects servers using ONNX model inference.
// Falls back to WeightedRoundRobin when the circuit breaker is open or the
// model service is unavailable.
type MLStrategy struct {
	endpoint string
	timeout  time.Duration
	client   *http.Client
	col      *metrics.Collector
	cb       *circuitBreaker
	fallback Strategy
	cache    *lru.Cache[string, []float32]
	logger   *zap.Logger
}

// NewMLStrategy constructs an MLStrategy from config.
func NewMLStrategy(cfg *config.MLConfig, col *metrics.Collector, logger *zap.Logger) *MLStrategy {
	cache, _ := lru.New[string, []float32](cfg.CacheSize)

	return &MLStrategy{
		endpoint: cfg.Endpoint,
		timeout:  time.Duration(cfg.TimeoutMs) * time.Millisecond,
		client:   &http.Client{Timeout: time.Duration(cfg.TimeoutMs) * time.Millisecond},
		col:      col,
		cb: newCircuitBreaker(
			time.Duration(cfg.CircuitBreakerResetSec) * time.Second,
		),
		fallback: NewWeightedRoundRobin(),
		cache:    cache,
		logger:   logger,
	}
}

// Select implements Strategy.
func (s *MLStrategy) Select(servers []*Server) *Server {
	if s.cb.isOpen() {
		s.col.SetCircuitOpen(true)
		return s.fallback.Select(servers)
	}
	s.col.SetCircuitOpen(false)

	cacheKey := buildCacheKey(servers, s.col)

	// Check cache first
	if cached, ok := s.cache.Get(cacheKey); ok {
		s.col.RecordMLCacheHit()
		return s.pickBest(servers, cached)
	}
	s.col.RecordMLCacheMiss()

	// Call ML model with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	start := time.Now()
	predictions, err := s.fetchPredictions(ctx, servers)
	elapsed := time.Since(start)

	if err != nil {
		s.logger.Warn("ML prediction failed, using fallback",
			zap.Error(err), zap.Duration("elapsed", elapsed))
		s.cb.recordFailure()
		s.col.RecordMLError()
		return s.fallback.Select(servers)
	}

	s.col.RecordMLInference(elapsed)
	s.cb.recordSuccess()
	s.cache.Add(cacheKey, predictions)

	if best := s.pickBest(servers, predictions); best != nil {
		return best
	}
	return s.fallback.Select(servers)
}

// mlRequest is the payload sent to the model server.
type mlRequest struct {
	Servers []serverFeatures `json:"servers"`
	Ts      int64            `json:"ts"`
}

type serverFeatures struct {
	ServerID    int     `json:"server_id"`
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	ActiveConns int     `json:"active_conns"`
	ErrorRate   float64 `json:"error_rate"`
	ResponseP95 float64 `json:"response_p95"`
	Weight      int     `json:"weight"`
	Capacity    int     `json:"capacity"`
}

// mlResponse matches what the Go model server returns.
// BUG FIX: was "predictions" in strategy but "scores" in model server.
// Both now use "predictions" — model server was updated accordingly.
type mlResponse struct {
	Predictions []float32 `json:"predictions"`
}

func (s *MLStrategy) fetchPredictions(ctx context.Context, servers []*Server) ([]float32, error) {
	features := make([]serverFeatures, len(servers))
	for i, srv := range servers {
		srv.mu.Lock()
		conns := srv.Connections
		srv.mu.Unlock()

		features[i] = serverFeatures{
			ServerID:    srv.ID,
			CPUUsage:    s.col.GetCPUUsage(srv.ID),
			MemoryUsage: s.col.GetMemoryUsage(srv.ID),
			ActiveConns: conns,
			ErrorRate:   s.col.GetErrorRate(srv.ID),
			ResponseP95: s.col.GetResponsePercentile(srv.ID, 0.95),
			Weight:      srv.Weight,
			Capacity:    srv.Capacity,
		}
	}

	payload := mlRequest{Servers: features, Ts: time.Now().Unix()}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+"/predict", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model service returned HTTP %d", resp.StatusCode)
	}

	var result mlResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if len(result.Predictions) != len(servers) {
		return nil, fmt.Errorf("prediction count mismatch: got %d, want %d",
			len(result.Predictions), len(servers))
	}

	return result.Predictions, nil
}

func (s *MLStrategy) pickBest(servers []*Server, predictions []float32) *Server {
	var best *Server
	bestScore := float32(math.MaxFloat32)

	for i, srv := range servers {
		if i >= len(predictions) {
			break
		}
		if srv.CanAcceptRequest() && predictions[i] < bestScore {
			bestScore = predictions[i]
			best = srv
		}
	}
	return best
}

// buildCacheKey creates a cache key from server IDs and current metrics.
// FIX: uses separator "|" to avoid hash collisions between different id/metric combos.
func buildCacheKey(servers []*Server, col *metrics.Collector) string {
	var b strings.Builder
	for _, s := range servers {
		fmt.Fprintf(&b, "%d|%.2f|%.2f|",
			s.ID,
			col.GetCPUUsage(s.ID),
			col.GetMemoryUsage(s.ID),
		)
	}
	return b.String()
}

// ─── Circuit Breaker ──────────────────────────────────────────────────────────

type circuitBreaker struct {
	mu          sync.Mutex
	open        bool
	lastFailure time.Time
	resetAfter  time.Duration
}

func newCircuitBreaker(resetAfter time.Duration) *circuitBreaker {
	return &circuitBreaker{resetAfter: resetAfter}
}

func (cb *circuitBreaker) isOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.open && time.Since(cb.lastFailure) >= cb.resetAfter {
		cb.open = false // auto-reset after window
	}
	return cb.open
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.open = true
	cb.lastFailure = time.Now()
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.open = false
}
