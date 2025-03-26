package loadbalancer

/*
import (
	"bytes"
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

func (s *MLStrategy) getPrediction(features []map[string]interface{}) (int, error) {
	payload, _ := json.Marshal(map[string]interface{}{
		"features":      features,
		"model_version": "v1",
	})

	resp, err := s.httpClient.Post(s.modelEndpoint+"/predict", "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return -1, fmt.Errorf("model service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("model service returned %d", resp.StatusCode)
	}

	var result struct {
		Prediction int `json:"prediction"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return -1, fmt.Errorf("invalid response format: %w", err)
	}

	return result.Prediction, nil
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

func (s *MLStrategy) SelectServer(servers []*Server) *Server {
	if s.circuitBreaker.IsOpen() {
		return s.fallbackStrategy.SelectServer(servers)
	}

	// Generate cache key
	cacheKey := generateCacheKey(servers)
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached.(*Server)
	}

	features := s.collectFeatures(servers)
	prediction, err := s.getPrediction(features)

	if err != nil {
		s.circuitBreaker.RecordFailure()
		return s.fallbackStrategy.SelectServer(servers)
	}

	s.circuitBreaker.RecordSuccess()

	// Find best server based on prediction
	var bestServer *Server
	minTime := float32(math.MaxFloat32)
	for _, server := range servers {
		if prediction[server.ID] < minTime {
			minTime = prediction[server.ID]
			bestServer = server
		}
	}

	if bestServer != nil {
		s.cache.Add(cacheKey, bestServer)
		return bestServer
	}

	return s.fallbackStrategy.SelectServer(servers)
}

func generateCacheKey(servers []*Server) string {
	var key strings.Builder
	for _, s := range servers {
		key.WriteString(fmt.Sprintf("%d-%.2f-%.2f",
			s.ID,
			s.metrics.GetCurrentCPUUsage(s.ID),
			s.metrics.GetCurrentMemoryUsage(s.ID),
		))
	}
	return key.String()
}
*/
