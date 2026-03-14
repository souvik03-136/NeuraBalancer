// File: backend/internal/metrics/collector.go
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/database"
)

// Collector owns all Prometheus metrics for the load balancer.
// It is a singleton — call NewCollector once and inject it everywhere.
type Collector struct {
	db     *database.DB
	logger *zap.Logger

	// HTTP traffic counters
	httpTotal    *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec

	// Per-server gauges (labeled by server_id)
	cpuUsage        *prometheus.GaugeVec
	memoryUsage     *prometheus.GaugeVec
	errorRate       *prometheus.GaugeVec
	activeConns     *prometheus.GaugeVec
	responseSummary *prometheus.SummaryVec

	// ML model metrics
	mlInferenceSeconds prometheus.Histogram
	mlPredictions      prometheus.Counter
	mlErrors           prometheus.Counter
	mlCacheHits        prometheus.Counter
	mlCacheMisses      prometheus.Counter
	mlCircuitOpen      prometheus.Gauge
}

// NewCollector creates and registers all Prometheus metrics.
// promauto handles registration; panics if a metric is registered twice —
// prevented by the singleton pattern.
func NewCollector(db *database.DB, logger *zap.Logger) *Collector {
	return &Collector{
		db:     db,
		logger: logger,

		httpTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "neurabalancer",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by method, path, status, and server.",
		}, []string{"method", "path", "status", "server_id"}),

		httpDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "neurabalancer",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"method", "path", "server_id"}),

		cpuUsage: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "neurabalancer",
			Name:      "server_cpu_usage_percent",
			Help:      "Current CPU usage percent for a backend server.",
		}, []string{"server_id"}),

		memoryUsage: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "neurabalancer",
			Name:      "server_memory_usage_percent",
			Help:      "Current memory usage percent for a backend server.",
		}, []string{"server_id"}),

		errorRate: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "neurabalancer",
			Name:      "server_error_rate",
			Help:      "Rolling error rate (0–1) for a backend server.",
		}, []string{"server_id"}),

		activeConns: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "neurabalancer",
			Name:      "server_active_connections",
			Help:      "Number of active in-flight connections to a backend server.",
		}, []string{"server_id"}),

		responseSummary: promauto.NewSummaryVec(prometheus.SummaryOpts{
			Namespace:  "neurabalancer",
			Name:       "server_response_duration_seconds",
			Help:       "Response time summary by server.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.005, 0.99: 0.001},
			MaxAge:     5 * time.Minute,
		}, []string{"server_id"}),

		mlInferenceSeconds: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "neurabalancer",
			Name:      "ml_inference_duration_seconds",
			Help:      "ML model inference latency.",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25},
		}),

		mlPredictions: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "neurabalancer",
			Name:      "ml_predictions_total",
			Help:      "Total successful ML predictions.",
		}),

		mlErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "neurabalancer",
			Name:      "ml_errors_total",
			Help:      "Total ML prediction errors (including circuit-breaker trips).",
		}),

		mlCacheHits: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "neurabalancer",
			Name:      "ml_cache_hits_total",
			Help:      "ML prediction cache hits.",
		}),

		mlCacheMisses: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "neurabalancer",
			Name:      "ml_cache_misses_total",
			Help:      "ML prediction cache misses.",
		}),

		mlCircuitOpen: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "neurabalancer",
			Name:      "ml_circuit_breaker_open",
			Help:      "1 if the ML circuit breaker is currently open, 0 otherwise.",
		}),
	}
}

// RecordRequest records a completed request for server serverID.
// It updates Prometheus metrics and writes to the database asynchronously.
func (c *Collector) RecordRequest(ctx context.Context, serverID int, method, path string, statusCode int, duration time.Duration, success bool) {
	sid := fmt.Sprint(serverID)
	statusStr := fmt.Sprint(statusCode)

	c.httpTotal.WithLabelValues(method, path, statusStr, sid).Inc()
	c.httpDuration.WithLabelValues(method, path, sid).Observe(duration.Seconds())
	c.responseSummary.WithLabelValues(sid).Observe(duration.Seconds())

	if !success {
		// Update error-rate gauge: read current value, blend it
		c.updateErrorRate(serverID)
	}

	// DB write is non-blocking — request handling must not be slowed by DB I/O
	go func() {
		if err := c.db.InsertRequest(ctx, serverID, success, duration); err != nil {
			c.logger.Warn("failed to insert request record", zap.Error(err), zap.Int("server_id", serverID))
		}
	}()
}

// UpdateServerMetrics fetches live CPU/memory from the backend server's
// /metrics endpoint and updates Prometheus gauges.
// Called from background goroutine, not the request path.
func (c *Collector) UpdateServerMetrics(ctx context.Context, serverID int, serverURL string) {
	cpu, mem, err := fetchServerMetrics(ctx, serverURL)
	if err != nil {
		c.logger.Debug("failed to fetch server metrics",
			zap.String("url", serverURL), zap.Error(err))
		return
	}

	sid := fmt.Sprint(serverID)
	c.cpuUsage.WithLabelValues(sid).Set(cpu)
	c.memoryUsage.WithLabelValues(sid).Set(mem)

	go func() {
		sr, err := c.db.SuccessRate(ctx, serverID, 5*time.Minute)
		if err != nil {
			sr = 1.0
		}
		if err := c.db.InsertMetrics(ctx, serverID, cpu, mem, 0, sr); err != nil {
			c.logger.Warn("failed to insert metrics", zap.Error(err))
		}
	}()
}

// SetActiveConnections updates the active-connection gauge for a server.
func (c *Collector) SetActiveConnections(serverID, count int) {
	c.activeConns.WithLabelValues(fmt.Sprint(serverID)).Set(float64(count))
}

// ML helpers

func (c *Collector) RecordMLInference(d time.Duration) {
	c.mlInferenceSeconds.Observe(d.Seconds())
	c.mlPredictions.Inc()
}

func (c *Collector) RecordMLError()     { c.mlErrors.Inc() }
func (c *Collector) RecordMLCacheHit()  { c.mlCacheHits.Inc() }
func (c *Collector) RecordMLCacheMiss() { c.mlCacheMisses.Inc() }
func (c *Collector) SetCircuitOpen(open bool) {
	if open {
		c.mlCircuitOpen.Set(1)
	} else {
		c.mlCircuitOpen.Set(0)
	}
}

// Feature extraction helpers (called by ML strategy)

func (c *Collector) GetCPUUsage(serverID int) float64 {
	return getGaugeValue(c.cpuUsage, fmt.Sprint(serverID))
}

func (c *Collector) GetMemoryUsage(serverID int) float64 {
	return getGaugeValue(c.memoryUsage, fmt.Sprint(serverID))
}

func (c *Collector) GetErrorRate(serverID int) float64 {
	return getGaugeValue(c.errorRate, fmt.Sprint(serverID))
}

func (c *Collector) GetResponsePercentile(serverID int, quantile float64) float64 {
	metricCh := make(chan prometheus.Metric, 1)
	go func() {
		c.responseSummary.Collect(metricCh)
		close(metricCh)
	}()
	for m := range metricCh {
		var dm dto.Metric
		if err := m.Write(&dm); err != nil || dm.Summary == nil {
			continue
		}
		for _, q := range dm.Summary.Quantile {
			if q.GetQuantile() == quantile {
				return q.GetValue()
			}
		}
	}
	return 0
}

// ─── private helpers ──────────────────────────────────────────────────────────

func (c *Collector) updateErrorRate(serverID int) {
	// Lightweight approximation: flip gauge toward 1 on error
	current := getGaugeValue(c.errorRate, fmt.Sprint(serverID))
	blended := 0.9*current + 0.1*1.0
	c.errorRate.WithLabelValues(fmt.Sprint(serverID)).Set(blended)
}

func getGaugeValue(gv *prometheus.GaugeVec, label string) float64 {
	g, err := gv.GetMetricWithLabelValues(label)
	if err != nil {
		return 0
	}
	var m dto.Metric
	if err := g.Write(&m); err != nil || m.Gauge == nil {
		return 0
	}
	return m.Gauge.GetValue()
}

// ServerMetricsResponse is what each backend server returns from /metrics.
type ServerMetricsResponse struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
}

func fetchServerMetrics(ctx context.Context, serverURL string) (cpu, mem float64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/metrics", nil)
	if err != nil {
		return 0, 0, err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("unexpected status %d from %s/metrics", resp.StatusCode, serverURL)
	}

	var result ServerMetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, fmt.Errorf("decode error: %w", err)
	}

	if result.CPUUsage < 0 || result.CPUUsage > 100 {
		return 0, 0, fmt.Errorf("invalid cpu_usage value: %v", result.CPUUsage)
	}
	if result.MemoryUsage < 0 || result.MemoryUsage > 100 {
		return 0, 0, fmt.Errorf("invalid memory_usage value: %v", result.MemoryUsage)
	}

	return result.CPUUsage, result.MemoryUsage, nil
}
