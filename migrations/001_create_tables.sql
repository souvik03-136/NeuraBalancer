-- Enable TimescaleDB extension (only if using TimescaleDB)
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Servers table (tracking backend nodes)
CREATE TABLE servers (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    ip_address TEXT UNIQUE NOT NULL,
    port INTEGER NOT NULL CHECK (port > 0 AND port < 65536),
    status TEXT CHECK (status IN ('active', 'inactive', 'down')) DEFAULT 'active',
    load INT DEFAULT 0 CHECK (load >= 0),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Requests log table
CREATE TABLE request_logs (
    id SERIAL PRIMARY KEY,
    server_id INTEGER REFERENCES servers(id) ON DELETE CASCADE,
    request_time TIMESTAMPTZ DEFAULT NOW(),
    response_time FLOAT NOT NULL CHECK (response_time >= 0),
    status_code INTEGER NOT NULL CHECK (status_code >= 100 AND status_code <= 599)
);

-- Metrics table for historical tracking (Hypertable for TimescaleDB)
CREATE TABLE metrics (
    id SERIAL PRIMARY KEY,
    server_id INT REFERENCES servers(id) ON DELETE CASCADE,
    cpu_usage FLOAT NOT NULL CHECK (cpu_usage >= 0 AND cpu_usage <= 100),
    memory_usage FLOAT NOT NULL CHECK (memory_usage >= 0),
    request_count INT NOT NULL CHECK (request_count >= 0),
    success_rate FLOAT NOT NULL CHECK (success_rate >= 0 AND success_rate <= 1),
    timestamp TIMESTAMPTZ DEFAULT NOW()
);

-- Convert the metrics table into a hypertable for efficient time-series querying
SELECT create_hypertable('metrics', 'timestamp');

-- Indexes for performance optimization
CREATE INDEX idx_request_logs_time ON request_logs(request_time DESC);
CREATE INDEX idx_metrics_server ON metrics(server_id, timestamp DESC);
