package metrics

import (
	"log"
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

// NewCollector initializes and returns a new Collector.
func NewCollector() *Collector {
	c := &Collector{
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
	prometheus.MustRegister(c.totalRequests, c.successfulRequests, c.failedRequests, c.responseTime)

	return c
}

// RecordRequest logs request data and updates server load.
func (c *Collector) RecordRequest(serverID int, success bool, duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.totalRequests.Inc()
	if success {
		c.successfulRequests.Inc()
	} else {
		c.failedRequests.Inc()
	}

	c.responseTime.Observe(duration.Seconds())

	// Store metrics in TimescaleDB
	if err := database.InsertRequest(serverID, success, duration); err != nil {
		log.Printf("❌ Failed to log request in database: %v", err)
	}

	// Update server load (increase by 1)
	err := database.UpdateServerLoad(serverID, 1)
	if err != nil {
		log.Printf("❌ Failed to update server load: %v", err)
	}
}
