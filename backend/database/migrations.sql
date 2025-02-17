-- Enable TimescaleDB extension (if not already enabled)
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Servers table (tracking backend nodes)
CREATE TABLE servers (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    ip_address TEXT UNIQUE NOT NULL,
    port INTEGER NOT NULL,
    status TEXT CHECK (status IN ('active', 'inactive', 'down')) DEFAULT 'active',
    load INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Requests log table
CREATE TABLE request_logs (
    id SERIAL PRIMARY KEY,
    server_id INTEGER REFERENCES servers(id) ON DELETE CASCADE,
    request_time TIMESTAMPTZ DEFAULT NOW(),
    response_time FLOAT NOT NULL,
    status_code INTEGER NOT NULL
);

-- Metrics table for historical tracking (Hypertable for TimescaleDB)
CREATE TABLE metrics (
    id SERIAL PRIMARY KEY,
    server_id INT REFERENCES servers(id) ON DELETE CASCADE,
    cpu_usage FLOAT NOT NULL,
    memory_usage FLOAT NOT NULL,
    request_count INT NOT NULL,
    success_rate FLOAT NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW()
);

-- Convert the metrics table into a hypertable for efficient time-series querying
SELECT create_hypertable('metrics', 'timestamp');

-- Index for optimizing query performance
CREATE INDEX idx_request_logs_time ON request_logs(request_time DESC);
