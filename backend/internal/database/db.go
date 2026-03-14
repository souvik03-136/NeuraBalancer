// File: backend/internal/database/db.go
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/config"
)

// DB is the application-level database handle. Obtain it via New().
type DB struct {
	pool   *sql.DB
	logger *zap.Logger
}

// New opens a PostgreSQL connection pool using the provided config.
func New(cfg *config.DatabaseConfig, log *zap.Logger) (*DB, error) {
	pool, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	pool.SetMaxOpenConns(cfg.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.MaxIdleConns)
	pool.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	log.Info("database connected", zap.String("host", cfg.Host), zap.Int("port", cfg.Port))
	return &DB{pool: pool, logger: log}, nil
}

// Close releases the connection pool.
func (d *DB) Close() error {
	return d.pool.Close()
}

// Pool exposes the raw sql.DB for migration tools.
func (d *DB) Pool() *sql.DB {
	return d.pool
}

// ─── Server registry ─────────────────────────────────────────────────────────

// UpsertServer registers or refreshes a server entry.
func (d *DB) UpsertServer(ctx context.Context, serverURL string) error {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL %q: %w", serverURL, err)
	}
	host := parsed.Hostname()
	portStr := parsed.Port()
	if portStr == "" {
		portStr = "80"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port in URL %q: %w", serverURL, err)
	}

	_, err = d.pool.ExecContext(ctx, `
		INSERT INTO servers (name, ip_address, port, status, is_active, created_at)
		VALUES ($1, $2, $3, 'active', TRUE, NOW())
		ON CONFLICT (ip_address, port)
		DO UPDATE SET status = 'active', is_active = TRUE, updated_at = NOW()
	`, serverURL, host, port)
	return err
}

// GetActiveServers returns all currently active server URLs.
func (d *DB) GetActiveServers(ctx context.Context) ([]string, error) {
	rows, err := d.pool.QueryContext(ctx, `
		SELECT name FROM servers WHERE is_active = TRUE ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

// GetServerID resolves a server URL to its integer primary key.
func (d *DB) GetServerID(ctx context.Context, serverURL string) (int, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return 0, err
	}
	var id int
	err = d.pool.QueryRowContext(ctx, `
		SELECT id FROM servers WHERE ip_address = $1 AND port = $2
	`, parsed.Hostname(), parsed.Port()).Scan(&id)
	return id, err
}

// UpdateServerStatus marks a server active or inactive.
func (d *DB) UpdateServerStatus(ctx context.Context, serverID int, isActive bool) error {
	_, err := d.pool.ExecContext(ctx, `
		UPDATE servers SET is_active = $1, updated_at = NOW() WHERE id = $2
	`, isActive, serverID)
	return err
}

// GetServerWeight returns the configured weight for a server (default 1).
func (d *DB) GetServerWeight(ctx context.Context, serverID int) (int, error) {
	var weight int
	err := d.pool.QueryRowContext(ctx, `
		SELECT COALESCE(weight, 1) FROM servers WHERE id = $1
	`, serverID).Scan(&weight)
	if err != nil {
		return 1, err
	}
	return weight, nil
}

// GetServerCapacity returns the configured capacity for a server (default 1).
func (d *DB) GetServerCapacity(ctx context.Context, serverID int) (int, error) {
	var capacity int
	err := d.pool.QueryRowContext(ctx, `
		SELECT COALESCE(capacity, 10) FROM servers WHERE id = $1
	`, serverID).Scan(&capacity)
	if err != nil {
		return 10, err
	}
	return capacity, nil
}

// ─── Metrics & requests ───────────────────────────────────────────────────────

// InsertRequest logs a completed request.
func (d *DB) InsertRequest(ctx context.Context, serverID int, success bool, responseTime time.Duration) error {
	_, err := d.pool.ExecContext(ctx, `
		INSERT INTO requests (server_id, status, response_time_ms, created_at)
		VALUES ($1, $2, $3, NOW())
	`, serverID, success, responseTime.Milliseconds())
	return err
}

// InsertMetrics records a server metrics snapshot.
func (d *DB) InsertMetrics(ctx context.Context, serverID int, cpu, mem float64, count int, successRate float64) error {
	_, err := d.pool.ExecContext(ctx, `
		INSERT INTO metrics (server_id, cpu_usage, memory_usage, request_count, success_rate, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, serverID, cpu, mem, count, successRate)
	return err
}

// SuccessRate calculates the request success rate for a server over the last window.
func (d *DB) SuccessRate(ctx context.Context, serverID int, window time.Duration) (float64, error) {
	var total, successes int
	err := d.pool.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = TRUE)
		FROM requests
		WHERE server_id = $1
		  AND created_at > NOW() - $2::interval
	`, serverID, fmt.Sprintf("%d seconds", int(window.Seconds()))).Scan(&total, &successes)
	if err != nil {
		return 1.0, err
	}
	if total == 0 {
		return 1.0, nil
	}
	return float64(successes) / float64(total), nil
}
