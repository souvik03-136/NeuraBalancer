package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/souvik03-136/neurabalancer/backend/database"
)

// Collector collects request metrics.
type Collector struct {
	mu                 sync.Mutex
	totalRequests      prometheus.Counter
	successfulRequests prometheus.Counter
	failedRequests     prometheus.Counter
	responseTime       prometheus.Histogram

	cpuUsage      *prometheus.GaugeVec
	memoryUsage   *prometheus.GaugeVec
	errorRate     *prometheus.GaugeVec
	responseTimes *prometheus.SummaryVec

	// ML Metrics Tracking
	mlInferenceTime prometheus.Histogram
	mlPredictions   prometheus.Counter
	mlErrors        prometheus.Counter
}

var (
	collectorInstance *Collector
	once              sync.Once
)

// NewCollector initializes and returns a new Collector (singleton).
func NewCollector() *Collector {
	once.Do(func() {
		collectorInstance = &Collector{
			totalRequests: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "http_total_requests",
				Help: "Total number of HTTP requests",
			}),
			successfulRequests: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "http_successful_requests",
				Help: "Number of successful HTTP requests",
			}),
			failedRequests: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "http_failed_requests",
				Help: "Number of failed HTTP requests",
			}),
			responseTime: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:    "http_response_time_seconds",
				Help:    "Histogram of response times",
				Buckets: prometheus.DefBuckets,
			}),
			cpuUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "server_cpu_usage",
				Help: "Current CPU usage percentage",
			}, []string{"server_id"}),

			memoryUsage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "server_memory_usage",
				Help: "Current memory usage percentage",
			}, []string{"server_id"}),

			errorRate: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "server_error_rate",
				Help: "Current error rate (0-1)",
			}, []string{"server_id"}),

			// Modified responseTimes metric with new name and help text.
			responseTimes: prometheus.NewSummaryVec(prometheus.SummaryOpts{
				Name:       "http_response_time_seconds_summary",
				Help:       "Response time percentiles (summary)",
				Objectives: map[float64]float64{0.5: 0.05, 0.95: 0.01},
			}, []string{"server_id"}),

			// Initialize ML metrics
			mlInferenceTime: prometheus.NewHistogram(prometheus.HistogramOpts{
				Name:    "ml_inference_time_seconds",
				Help:    "Histogram of ML model inference times",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			}),
			mlPredictions: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "ml_predictions_total",
				Help: "Total number of ML model predictions",
			}),
			mlErrors: prometheus.NewCounter(prometheus.CounterOpts{
				Name: "ml_errors_total",
				Help: "Total number of ML model errors",
			}),
		}

		// Register metrics with Prometheus
		prometheus.MustRegister(
			collectorInstance.totalRequests,
			collectorInstance.successfulRequests,
			collectorInstance.failedRequests,
			collectorInstance.responseTime,

			collectorInstance.cpuUsage,
			collectorInstance.memoryUsage,
			collectorInstance.errorRate,
			collectorInstance.responseTimes,

			// Register ML metrics
			collectorInstance.mlInferenceTime,
			collectorInstance.mlPredictions,
			collectorInstance.mlErrors,
		)
	})

	return collectorInstance
}

// RecordRequest records a request's metrics and updates the database.
func (c *Collector) RecordRequest(serverID int, success bool, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update Prometheus metrics FIRST
	c.totalRequests.Inc()
	if success {
		c.successfulRequests.Inc()
	} else {
		c.failedRequests.Inc()
	}
	c.responseTime.Observe(duration.Seconds())

	// Validate serverID before proceeding
	if serverID <= 0 {
		log.Printf(" Invalid server ID: %d. Skipping metrics insertion.", serverID)
		return
	}

	// Verify server existence
	exists, err := database.ServerExists(serverID)
	if err != nil {
		log.Printf(" Error checking server existence (ID %d): %v", serverID, err)
		return
	}
	if !exists {
		log.Printf(" Server ID %d not found in database. Skipping metrics.", serverID)
		return
	}

	// Store request IMMEDIATELY (critical for success rate accuracy)
	if err := database.InsertRequest(serverID, success, duration); err != nil {
		log.Printf(" Failed to log request (Server %d): %v", serverID, err)
	}

	// Update server load
	if err := database.UpdateServerLoad(serverID, 1); err != nil {
		log.Printf(" Failed to update load (Server %d): %v", serverID, err)
	}

	// Get server-specific metrics (avoid system fallback)
	cpuUsage, memoryUsage, err := getActualServerMetrics(serverID)
	if err != nil {
		log.Printf(" Failed to get metrics for server %d: %v", serverID, err)
		cpuUsage = 0.0 // Explicit default
		memoryUsage = 0.0
	}

	// Clamp metrics BEFORE insertion
	cpuUsage = clamp(cpuUsage, 0, 100)
	memoryUsage = clamp(memoryUsage, 0, 100)

	// Calculate success rate AFTER request is stored
	successRate, err := calculateSuccessRate(serverID)
	if err != nil {
		log.Printf(" Failed to calculate success rate for server %d: %v", serverID, err)
		successRate = 1.0 // Conservative fallback
	}
	successRate = clamp(successRate, 0, 1) // Ensure 0-1 range

	// Insert metrics
	if err := database.InsertMetrics(
		serverID,
		cpuUsage,
		memoryUsage,
		1,
		successRate,
	); err != nil {
		log.Printf(" Failed to insert metrics (Server %d): %v", serverID, err)
	}

	// Store in both tables
	if err := database.InsertRequest(serverID, success, duration); err != nil {
		log.Printf("Failed to log request: %v", err)
	}

	// Create a background context for the InsertAttempt call
	ctx := context.Background()
	if err := database.InsertAttempt(ctx, serverID, success); err != nil {
		log.Printf("Failed to log attempt: %v", err)
	}
}

// Helper function to clamp values between min and max
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// getActualServerMetrics fetches CPU and memory usage from the backend server's /metrics endpoint.
func getActualServerMetrics(serverID int) (float64, float64, error) {
	var serverURL string
	err := database.DB.QueryRow(
		"SELECT name FROM servers WHERE id = $1",
		serverID,
	).Scan(&serverURL)
	if err != nil {
		return 0, 0, fmt.Errorf("server URL lookup failed: %v", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/metrics", serverURL))
	if err != nil {
		return 0, 0, fmt.Errorf("metrics endpoint unreachable: %v", err)
	}
	defer resp.Body.Close()

	// Validate response status code
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("non-200 status code: %d", resp.StatusCode)
	}

	// Parse JSON response
	var metrics struct {
		CPU    float64 `json:"cpu_usage"`
		Memory float64 `json:"memory_usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return 0, 0, fmt.Errorf("metrics parsing failed: %v", err)
	}

	// Validate metrics values
	if metrics.CPU < 0 || metrics.CPU > 100 {
		return 0, 0, fmt.Errorf("invalid CPU usage value: %.2f", metrics.CPU)
	}
	if metrics.Memory < 0 || metrics.Memory > 100 {
		return 0, 0, fmt.Errorf("invalid memory usage value: %.2f", metrics.Memory)
	}

	return metrics.CPU, metrics.Memory, nil
}

// calculateSuccessRate calculates the success rate for a server based on recent requests.
func calculateSuccessRate(serverID int) (float64, error) {
	var attempts, successes int
	err := database.DB.QueryRow(`
        SELECT 
            COUNT(*) FILTER (WHERE server_id = $1),
            COUNT(*) FILTER (WHERE server_id = $1 AND success = true)
        FROM attempts 
        WHERE timestamp > NOW() - INTERVAL '5 minutes'`,
		serverID,
	).Scan(&attempts, &successes)

	if attempts == 0 {
		return 1.0, nil // Assume 100% success if no data
	}
	return float64(successes) / float64(attempts), err
}

// ML Metrics recording methods

// RecordMLInference records ML inference time
func (c *Collector) RecordMLInference(duration time.Duration) {
	c.mlInferenceTime.Observe(duration.Seconds())
	c.mlPredictions.Inc()
}

// RecordMLError increments the ML error counter
func (c *Collector) RecordMLError() {
	c.mlErrors.Inc()
}

// FOR ML feature extraction methods

// GetCurrentCPUUsage gets the current CPU usage for a server
func (c *Collector) GetCurrentCPUUsage(serverID int) float64 {
	gauge, err := c.cpuUsage.GetMetricWithLabelValues(fmt.Sprint(serverID))
	if err != nil {
		return 0
	}

	var m dto.Metric
	if err := gauge.Write(&m); err != nil {
		return 0
	}

	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	return 0
}

// GetCurrentMemoryUsage gets the current memory usage for a server
func (c *Collector) GetCurrentMemoryUsage(serverID int) float64 {
	gauge, err := c.memoryUsage.GetMetricWithLabelValues(fmt.Sprint(serverID))
	if err != nil {
		return 0
	}

	var m dto.Metric
	if err := gauge.Write(&m); err != nil {
		return 0
	}

	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	return 0
}

// GetErrorRate gets the current error rate for a server
func (c *Collector) GetErrorRate(serverID int) float64 {
	gauge, err := c.errorRate.GetMetricWithLabelValues(fmt.Sprint(serverID))
	if err != nil {
		return 0
	}

	var m dto.Metric
	if err := gauge.Write(&m); err != nil {
		return 0
	}

	if m.Gauge != nil {
		return m.Gauge.GetValue()
	}
	return 0
}

// GetResponsePercentile gets a specific response time percentile for a server
func (c *Collector) GetResponsePercentile(serverID int, percentile float64) float64 {
	metricChan := make(chan prometheus.Metric, 1)
	go func() {
		c.responseTimes.Collect(metricChan)
		close(metricChan)
	}()

	for metric := range metricChan {
		var m dto.Metric
		if err := metric.Write(&m); err != nil {
			continue
		}

		if m.Summary == nil {
			continue
		}

		for _, q := range m.Summary.Quantile {
			if q.GetQuantile() == percentile {
				return q.GetValue()
			}
		}
	}

	return 0
}
