package database

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
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
	_, err := DB.Exec(
		`INSERT INTO metrics (server_id, cpu_usage, memory_usage, request_count, success_rate, timestamp)
		VALUES ($1, $2, $3, $4, $5, NOW())`,
		serverID, cpu, mem, count, successRate,
	)
	if err != nil {
		log.Printf(" Failed to insert metrics for server ID %d: %v", serverID, err)
		return err
	}
	log.Printf(" Metrics inserted successfully for server ID %d", serverID)
	return nil
}

// UpdateServerStatus updates the is_active status of a server.
func UpdateServerStatus(serverID int, isActive bool) error {
	const maxRetries = 3
	var err error

	for i := 0; i < maxRetries; i++ {
		_, err = DB.Exec(`
		UPDATE servers 
		SET is_active = $1,
			last_checked = NOW() 
		WHERE id = $2`,
			isActive, serverID,
		)

		if err == nil {
			return nil
		}

		log.Printf(" DB update attempt %d/%d failed: %v", i+1, maxRetries, err)
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("failed to update server status after %d attempts: %v", maxRetries, err)
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

// RegisterServer registers a server in the database.
func RegisterServer(server string) error {
	// Parse the server URL
	parsed, err := url.Parse(server)
	if err != nil {
		log.Printf(" Invalid server URL: %s, error: %v", server, err)
		return err
	}

	// Extract IP address and port
	ipAddress := parsed.Hostname()
	portStr := parsed.Port()
	if portStr == "" {
		portStr = "80" // Default to port 80 if no port is specified
	}

	// Convert port to integer
	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Printf(" Invalid port number: %s", portStr)
		return err
	}

	query := `
		INSERT INTO servers (name, ip_address, port, status, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (ip_address, port) 
		DO UPDATE SET status = EXCLUDED.status, is_active = EXCLUDED.is_active
	`

	// Execute the query with the correct parameters
	_, err = DB.Exec(query, server, ipAddress, port, "active", true)
	if err != nil {
		log.Printf(" Failed to register/update server in DB: %v", err)
		return err
	}

	log.Printf(" Server %s registered/updated successfully", server)
	return nil
}

// ServerExists checks if a server with the given ID exists in the database
// ServerExists checks if a server with the given ID exists in the database
func ServerExists(serverID int) (bool, error) {
	var exists bool
	err := DB.QueryRow("SELECT EXISTS(SELECT 1 FROM servers WHERE id = $1)", serverID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func GetServerID(serverURL string) (int, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return -1, err
	}

	var serverID int
	err = DB.QueryRow(`
        SELECT id FROM servers 
        WHERE ip_address = $1 AND port = $2`,
		parsed.Hostname(), parsed.Port(),
	).Scan(&serverID)

	return serverID, err
}

// GetServerWeight retrieves the weight of a server from database
func GetServerWeight(serverID int) (int, error) {
	var weight int
	err := DB.QueryRow(`
        SELECT weight FROM servers 
        WHERE id = $1`,
		serverID,
	).Scan(&weight)

	if err != nil {
		log.Printf(" Error getting weight for server %d: %v", serverID, err)
		return 1, err // Return default weight 1 if not found
	}
	return weight, nil
}

func GetServerActiveStatus(serverID int) (bool, error) {
	var isActive bool
	err := DB.QueryRow(
		"SELECT is_active FROM servers WHERE id = $1",
		serverID,
	).Scan(&isActive)
	return isActive, err
}

// method to get server capacity
func GetServerCapacity(serverID int) (int, error) {
	var capacity int
	err := DB.QueryRow(`
        SELECT capacity FROM servers 
        WHERE id = $1`,
		serverID,
	).Scan(&capacity)

	if err != nil {
		log.Printf("Error getting capacity for server %d: %v", serverID, err)
		return 1, err // Default capacity 1 if not found
	}
	return capacity, nil
}

// Track all attempts including retries
func InsertAttempt(ctx context.Context, serverID int, success bool) error {
	_, err := DB.ExecContext(ctx, `
        INSERT INTO attempts 
            (server_id, success, timestamp)
        VALUES ($1, $2, NOW())`,
		serverID, success,
	)
	return err
}
