-- Enable TimescaleDB extension (if not already enabled)
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Servers table (tracking backend nodes)
CREATE TABLE servers (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    ip_address TEXT NOT NULL,
    port INTEGER NOT NULL,
    status TEXT CHECK (status IN ('active', 'inactive', 'down')) DEFAULT 'active',
    load INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_checked TIMESTAMPTZ DEFAULT NOW(), -- Added last_checked column
    weight INT DEFAULT 1, -- Added weight
    capacity INT DEFAULT 1 -- Added capacity
);

-- Requests table (logging requests per server)
CREATE TABLE requests (
    id SERIAL PRIMARY KEY,
    server_id INTEGER REFERENCES servers(id) ON DELETE CASCADE,
    status BOOLEAN NOT NULL, -- Changed from 'request_logs' table
    response_time FLOAT NOT NULL,
    timestamp TIMESTAMPTZ DEFAULT NOW()
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
CREATE INDEX idx_requests_time ON requests(timestamp DESC);
CREATE INDEX idx_metrics_time ON metrics(timestamp DESC);
