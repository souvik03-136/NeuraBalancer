package database

import (
	"log"
	"strings"
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

// GetLeastLoadedServer fetches the least loaded active server.
func GetLeastLoadedServer() (string, error) {
	var ip string
	err := DB.QueryRow(`
        SELECT ip_address FROM servers 
        WHERE is_active = TRUE 
        ORDER BY load ASC 
        LIMIT 1`).Scan(&ip)
	return ip, err
}

// InsertMetrics logs server metrics.
func InsertMetrics(serverID int, cpu float64, mem float64, count int, successRate float64) error {
	_, err := DB.Exec(`
        INSERT INTO metrics (server_id, cpu_usage, memory_usage, request_count, success_rate, timestamp)
        VALUES ($1, $2, $3, $4, $5, NOW())`,
		serverID, cpu, mem, count, successRate,
	)
	return err
}

// UpdateServerStatus updates the is_active status of a server.
func UpdateServerStatus(serverID int, isActive bool) error {
	_, err := DB.Exec(`
		UPDATE servers
		SET is_active = $1
		WHERE id = $2`,
		isActive, serverID,
	)
	if err != nil {
		log.Printf("❌ Failed to update server status for server ID %d: %v", serverID, err)
	}
	return err
}

// UpdateServerLoad updates the load of a specific server.
func UpdateServerLoad(serverID int, load int) error {
	_, err := DB.Exec(`
		UPDATE servers 
		SET load = $1 
		WHERE id = $2`,
		load, serverID,
	)
	return err
}
func GetAvailableServers() ([]string, error) {
	rows, err := DB.Query(`SELECT name FROM servers WHERE is_active = TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		servers = append(servers, url)
	}
	return servers, nil
}

func RegisterServer(server string) error {
	// Split into parts like ip, port, etc.
	parts := strings.Split(server, ":")
	ipAddress := parts[0]
	port := parts[1]
	query := `
	INSERT INTO servers (name, ip_address, port, status, is_active, created_at)
	VALUES ($1, $2, $3, $4, $5, NOW())
	ON CONFLICT (ip_address, port) 
	DO UPDATE SET status = EXCLUDED.status, is_active = EXCLUDED.is_active`
	_, err := DB.Exec(query, server, ipAddress, port, "active", 0, true)
	return err
}
