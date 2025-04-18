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

	lru "github.com/hashicorp/golang-lru"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

type MLStrategy struct {
	modelEndpoint    string
	httpClient       *http.Client
	metrics          *metrics.Collector
	circuitBreaker   *CircuitBreaker
	fallbackStrategy Strategy
	cache            *lru.Cache
}

type CircuitBreaker struct {
	mu          sync.Mutex
	isOpen      bool
	lastFailure time.Time
}

func NewMLStrategy(endpoint string, collector *metrics.Collector) *MLStrategy {
	cache, _ := lru.New(1000) // Cache 1000 predictions
	return &MLStrategy{
		modelEndpoint:    endpoint,
		httpClient:       &http.Client{Timeout: 300 * time.Millisecond},
		metrics:          collector,
		circuitBreaker:   &CircuitBreaker{},
		fallbackStrategy: &WeightedRoundRobinStrategy{},
		cache:            cache,
	}
}

func (s *MLStrategy) collectFeatures(servers []*Server) []map[string]interface{} {
	features := make([]map[string]interface{}, len(servers))
	for i, server := range servers {
		features[i] = map[string]interface{}{
			"server_id":    server.ID,
			"cpu_usage":    s.metrics.GetCurrentCPUUsage(server.ID),
			"memory_usage": s.metrics.GetCurrentMemoryUsage(server.ID),
			"active_conns": server.Connections,
			"error_rate":   s.metrics.GetErrorRate(server.ID),
			"response_p95": s.metrics.GetResponsePercentile(server.ID, 0.95),
			"weight":       server.Weight,
			"capacity":     server.Capacity,
		}
	}
	return features
}

// getPredictions sends a batch request to the ML model endpoint and expects
// a slice of float32 predictions (one per server).
func (s *MLStrategy) getPredictions(payload map[string]interface{}) ([]float32, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := s.httpClient.Post(s.modelEndpoint+"/predict", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("model service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model service returned %d", resp.StatusCode)
	}

	var result struct {
		Predictions []float32 `json:"predictions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("invalid response format: %w", err)
	}

	return result.Predictions, nil
}

// SelectServer chooses the best server using ML model predictions.
// It uses a timeout context, circuit breaker, retry logic, and caching.
func (s *MLStrategy) SelectServer(servers []*Server) *Server {
	// Check circuit breaker status.
	if s.circuitBreaker.IsOpen() {
		return s.fallbackStrategy.SelectServer(servers)
	}

	// Generate cache key from server metrics
	cacheKey := generateCacheKey(servers, s.metrics)

	// Check if we have a cached prediction
	if cachedPredictions, found := s.cache.Get(cacheKey); found {
		predictions, ok := cachedPredictions.([]float32)
		if ok && len(predictions) == len(servers) {
			// Use cached predictions to select server
			var (
				bestScore  = float32(math.MaxFloat32)
				bestServer *Server
			)
			for i, server := range servers {
				if score := predictions[i]; score < bestScore && server.CanHandleRequest() {
					bestScore = score
					bestServer = server
				}
			}

			if bestServer != nil {
				return bestServer
			}
		}
	}

	// Create a timeout context for the model call.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create batch request payload.
	features := s.collectFeatures(servers)
	payload := map[string]interface{}{
		"servers": features,
		"ts":      time.Now().Unix(),
	}

	// Add retry logic for model calls.
	var predictions []float32
	err := retry(3, 100*time.Millisecond, func() error {
		// Marshal payload.
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}
		// Create new request with the timeout context.
		req, err := http.NewRequestWithContext(ctx, "POST", s.modelEndpoint+"/predict", bytes.NewBuffer(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("model service unreachable: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("model service returned %d", resp.StatusCode)
		}

		var result struct {
			Predictions []float32 `json:"predictions"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("invalid response format: %w", err)
		}
		predictions = result.Predictions
		return nil
	})

	// Handle fallback on error.
	if err != nil || len(predictions) != len(servers) {
		s.circuitBreaker.RecordFailure()
		return s.fallbackStrategy.SelectServer(servers)
	}

	// Store predictions in cache for future use
	s.cache.Add(cacheKey, predictions)

	// Reset circuit breaker on success
	s.circuitBreaker.RecordSuccess()

	// Find server with best score (lowest prediction) that can handle the request.
	var (
		bestScore  = float32(math.MaxFloat32)
		bestServer *Server
	)
	for i, server := range servers {
		if score := predictions[i]; score < bestScore && server.CanHandleRequest() {
			bestScore = score
			bestServer = server
		}
	}

	if bestServer != nil {
		return bestServer
	}
	return s.fallbackStrategy.SelectServer(servers)
}

func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.isOpen && time.Since(cb.lastFailure) < 30*time.Second
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.isOpen = true
	cb.lastFailure = time.Now()
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.isOpen = false
}

// retry is a helper function that retries the given function up to 'attempts'
// times with the specified delay between attempts.
func retry(attempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return err
}

// generateCacheKey creates a unique key based on server metrics for caching.
func generateCacheKey(servers []*Server, collector *metrics.Collector) string {
	var key strings.Builder
	for _, s := range servers {
		key.WriteString(fmt.Sprintf("%d-%.2f-%.2f",
			s.ID,
			collector.GetCurrentCPUUsage(s.ID),
			collector.GetCurrentMemoryUsage(s.ID),
		))
	}
	return key.String()
}

// CanHandleRequest checks if the server can accept more requests.
// It allows up to 2x over-provisioning.
func (s *Server) CanHandleRequest() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Connections < s.Capacity*2
}
