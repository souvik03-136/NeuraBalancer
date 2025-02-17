package database

import (
	"time"
)

// InsertRequest logs a request into the database.
func InsertRequest(serverID int, success bool, responseTime time.Duration) error {
	_, err := DB.Exec(`
        INSERT INTO requests (server_id, status, response_time, timestamp)
        VALUES ($1, $2, $3, NOW())`,
		serverID, success, responseTime.Milliseconds(),
	)
	return err
}

// GetLeastLoadedServer fetches the least loaded server
func GetLeastLoadedServer() (string, error) {
	var ip string
	err := DB.QueryRow(`
        SELECT ip_address FROM servers WHERE is_active = TRUE ORDER BY load ASC LIMIT 1
    `).Scan(&ip)
	return ip, err
}

// InsertMetrics logs server metrics
func InsertMetrics(serverID int, cpu float64, mem float64, count int, successRate float64) error {
	_, err := DB.Exec(`
        INSERT INTO metrics (server_id, cpu_usage, memory_usage, request_count, success_rate, timestamp)
        VALUES ($1, $2, $3, $4, $5, NOW())`,
		serverID, cpu, mem, count, successRate,
	)
	return err
}
