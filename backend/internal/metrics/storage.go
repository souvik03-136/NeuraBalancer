package metrics

import (
	"database/sql"
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
func InsertMetric(serverID int, cpuUsage, memoryUsage float64, requestCount int, successRate float64) error {
	query := `INSERT INTO metrics (server_id, cpu_usage, memory_usage, request_count, success_rate) VALUES ($1, $2, $3, $4, $5)`
	_, err := DB.Exec(query, serverID, cpuUsage, memoryUsage, requestCount, successRate)
	if err != nil {
		log.Println("❌ Failed to insert metric:", err)
		return err
	}
	log.Println("✅ Metric inserted successfully")
	return nil
}

// GetMetrics retrieves all metrics for a specific server within a time range
func GetMetrics(serverID int, startTime, endTime time.Time) ([]Metric, error) {
	query := `SELECT id, server_id, cpu_usage, memory_usage, request_count, success_rate, timestamp 
	          FROM metrics WHERE server_id = $1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp DESC`
	rows, err := DB.Query(query, serverID, startTime, endTime)
	if err != nil {
		log.Println("❌ Failed to retrieve metrics:", err)
		return nil, err
	}
	defer rows.Close()

	var metrics []Metric
	for rows.Next() {
		var m Metric
		if err := rows.Scan(&m.ID, &m.ServerID, &m.CPUUsage, &m.MemoryUsage, &m.RequestCount, &m.SuccessRate, &m.Timestamp); err != nil {
			return nil, err
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
			log.Println("⚠️ No metrics found for server ID:", serverID)
			return nil, nil
		}
		log.Println("❌ Error retrieving latest metric:", err)
		return nil, err
	}
	return &m, nil
}
