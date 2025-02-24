package metrics

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/souvik03-136/neurabalancer/backend/database"
)

// Collector collects request metrics.
type Collector struct {
	mu                 sync.Mutex
	totalRequests      prometheus.Counter
	successfulRequests prometheus.Counter
	failedRequests     prometheus.Counter
	responseTime       prometheus.Histogram
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
		}

		// Register metrics with Prometheus
		prometheus.MustRegister(
			collectorInstance.totalRequests,
			collectorInstance.successfulRequests,
			collectorInstance.failedRequests,
			collectorInstance.responseTime,
		)
	})

	return collectorInstance
}
func (c *Collector) RecordRequest(serverID int, success bool, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update Prometheus metrics
	c.totalRequests.Inc()
	if success {
		c.successfulRequests.Inc()
	} else {
		c.failedRequests.Inc()
	}
	c.responseTime.Observe(duration.Seconds())

	// Validate serverID before proceeding
	if serverID <= 0 {
		log.Printf("⚠️ Invalid server ID: %d. Skipping metrics insertion.", serverID)
		return
	}

	// Verify server existence
	exists, err := database.ServerExists(serverID)
	if err != nil {
		log.Printf("❌ Error checking server existence (ID %d): %v", serverID, err)
		return
	}
	if !exists {
		log.Printf("⚠️ Server ID %d not found in database. Skipping metrics.", serverID)
		return
	}

	// Get actual server metrics
	cpuUsage, memoryUsage, err := getActualServerMetrics(serverID)
	if err != nil {
		log.Printf("⚠️ Failed to get metrics for server %d: %v", serverID, err)
		// Use safe defaults but still record the request
		cpuUsage = 0.0
		memoryUsage = 0.0
	}

	// Calculate dynamic success rate
	successRate, err := calculateSuccessRate(serverID)
	if err != nil {
		log.Printf("⚠️ Failed to calculate success rate for server %d: %v", serverID, err)
		successRate = 1.0 // Fallback value
	}

	// Store request in TimescaleDB
	if err := database.InsertRequest(serverID, success, duration); err != nil {
		log.Printf("❌ Failed to log request (Server %d): %v", serverID, err)
	}

	// Update server load
	if err := database.UpdateServerLoad(serverID, 1); err != nil {
		log.Printf("❌ Failed to update load (Server %d): %v", serverID, err)
	}

	// Insert comprehensive metrics
	if err := database.InsertMetrics(
		serverID,
		cpuUsage,
		memoryUsage,
		1, // Each request counts as 1
		successRate,
	); err != nil {
		log.Printf("❌ Failed to insert metrics (Server %d): %v", serverID, err)
	}
}

// Helper function to fetch actual server metrics
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

	var metrics struct {
		CPU    float64 `json:"cpu_usage"`
		Memory float64 `json:"memory_usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return 0, 0, fmt.Errorf("metrics parsing failed: %v", err)
	}

	return metrics.CPU, metrics.Memory, nil
}

// Helper function to calculate success rate
func calculateSuccessRate(serverID int) (float64, error) {
	var successCount, totalCount int
	err := database.DB.QueryRow(`
		SELECT 
			COUNT(*) FILTER (WHERE status = true),
			COUNT(*)
		FROM requests 
		WHERE server_id = $1
		AND timestamp > NOW() - INTERVAL '15 minutes'`,
		serverID,
	).Scan(&successCount, &totalCount)

	if err != nil {
		return 0, fmt.Errorf("database query failed: %v", err)
	}

	if totalCount == 0 {
		return 1.0, nil // Default to 100% if no recent data
	}

	return float64(successCount) / float64(totalCount), nil
}
