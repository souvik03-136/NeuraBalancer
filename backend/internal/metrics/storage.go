package metrics

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// DB is the global database connection instance
var DB *sql.DB

// Metric represents a server's performance metrics
type Metric struct {
	ID           int       `json:"id"`
	ServerID     int       `json:"server_id"`
	CPUUsage     float64   `json:"cpu_usage"`
	MemoryUsage  float64   `json:"memory_usage"`
	RequestCount int       `json:"request_count"`
	SuccessRate  float64   `json:"success_rate"`
	Timestamp    time.Time `json:"timestamp"`
}

// InsertMetric logs server performance metrics into the database
// InsertMetric inserts metrics for a server only if the server is active
func InsertMetric(serverID int, cpuUsage, memoryUsage float64, requestCount int, successRate float64) error {
	// Check if the server is active before inserting the metric
	var isActive bool
	err := DB.QueryRow(`SELECT is_active FROM servers WHERE id = $1`, serverID).Scan(&isActive)
	if err != nil {
		log.Printf("❌ Error checking server status: %v", err)
		return fmt.Errorf("error checking server status: %w", err)
	}

	// If the server is not active, skip inserting the metric
	if !isActive {
		log.Printf("❌ Server %d is not active. Skipping metrics insertion.", serverID)
		return nil
	}

	// Get the current timestamp
	timestamp := time.Now()

	// Insert the metric if the server is active
	query := `INSERT INTO metrics (server_id, cpu_usage, memory_usage, request_count, success_rate, timestamp) 
	          VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = DB.Exec(query, serverID, cpuUsage, memoryUsage, requestCount, successRate, timestamp)
	if err != nil {
		log.Printf("❌ Failed to insert metric: %v", err)
		return fmt.Errorf("failed to insert metric: %w", err)
	}

	log.Printf("✅ Metric inserted successfully for server %d", serverID)
	return nil
}

// GetMetrics retrieves all metrics for a specific server within a time range
func GetMetrics(serverID int, startTime, endTime time.Time) ([]Metric, error) {
	// Use default time range if not provided
	if startTime.IsZero() {
		startTime = time.Now().Add(-24 * time.Hour) // Default to the last 24 hours
	}

	query := `SELECT id, server_id, cpu_usage, memory_usage, request_count, success_rate, timestamp 
	          FROM metrics WHERE server_id = $1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp DESC`
	rows, err := DB.Query(query, serverID, startTime, endTime)
	if err != nil {
		log.Printf("❌ Failed to retrieve metrics: %v", err)
		return nil, fmt.Errorf("failed to retrieve metrics: %w", err)
	}
	defer rows.Close() // Ensure rows are closed when done

	var metrics []Metric
	for rows.Next() {
		var m Metric
		if err := rows.Scan(&m.ID, &m.ServerID, &m.CPUUsage, &m.MemoryUsage, &m.RequestCount, &m.SuccessRate, &m.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan metric row: %w", err)
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// GetLatestMetric retrieves the most recent metric entry for a given server
func GetLatestMetric(serverID int) (*Metric, error) {
	query := `SELECT id, server_id, cpu_usage, memory_usage, request_count, success_rate, timestamp 
	          FROM metrics WHERE server_id = $1 ORDER BY timestamp DESC LIMIT 1`
	row := DB.QueryRow(query, serverID)

	var m Metric
	if err := row.Scan(&m.ID, &m.ServerID, &m.CPUUsage, &m.MemoryUsage, &m.RequestCount, &m.SuccessRate, &m.Timestamp); err != nil {
		if err == sql.ErrNoRows {
			log.Printf("⚠️ No metrics found for server ID: %d", serverID)
			return nil, nil
		}
		log.Printf("❌ Error retrieving latest metric: %v", err)
		return nil, fmt.Errorf("error retrieving latest metric: %w", err)
	}
	return &m, nil
}
