package models

import (
	"database/sql"
	"time"
)

// RequestLog represents an API request log entry.
type RequestLog struct {
	ID           int       `json:"id"`
	ServerID     int       `json:"server_id"`
	RequestTime  time.Time `json:"request_time"`
	ResponseTime float64   `json:"response_time"`
	StatusCode   int       `json:"status_code"`
}

// CreateRequestLog inserts a new request log entry into the database.
func CreateRequestLog(db *sql.DB, serverID int, responseTime float64, statusCode int) error {
	query := `
		INSERT INTO request_logs (server_id, request_time, response_time, status_code)
		VALUES ($1, NOW(), $2, $3)
	`
	_, err := db.Exec(query, serverID, responseTime, statusCode)
	return err
}

// GetRequestsByServer retrieves request logs for a specific server.
func GetRequestsByServer(db *sql.DB, serverID int, limit int) ([]RequestLog, error) {
	query := `
		SELECT id, server_id, request_time, response_time, status_code
		FROM request_logs
		WHERE server_id = $1
		ORDER BY request_time DESC
		LIMIT $2
	`

	rows, err := db.Query(query, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []RequestLog
	for rows.Next() {
		var r RequestLog
		err := rows.Scan(&r.ID, &r.ServerID, &r.RequestTime, &r.ResponseTime, &r.StatusCode)
		if err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}

	return requests, nil
}

// DeleteOldRequests deletes logs older than a specified duration.
func DeleteOldRequests(db *sql.DB, olderThanDays int) error {
	query := `
		DELETE FROM request_logs
		WHERE request_time < NOW() - INTERVAL '1 day' * $1
	`
	_, err := db.Exec(query, olderThanDays)
	return err
}
