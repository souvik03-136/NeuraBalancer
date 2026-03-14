-- File: backend/migrations/001_initial_schema.up.sql
-- Initial schema for NeuraBalancer.
-- TimescaleDB hypertables are used for time-series tables.

CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- Servers registry
CREATE TABLE IF NOT EXISTS servers (
    id          SERIAL PRIMARY KEY,
    name        TEXT        NOT NULL,            -- full URL, e.g. http://backend-1:8001
    ip_address  TEXT        NOT NULL,
    port        INTEGER     NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'active',
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    weight      INTEGER     NOT NULL DEFAULT 1,
    capacity    INTEGER     NOT NULL DEFAULT 10,  -- max concurrent requests
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ip_address, port)
);

-- Per-request log (hypertable for time-based querying)
CREATE TABLE IF NOT EXISTS requests (
    id              BIGSERIAL,
    server_id       INTEGER     NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    status          BOOLEAN     NOT NULL,        -- TRUE = success
    response_time_ms BIGINT     NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SELECT create_hypertable('requests', 'created_at', if_not_exists => TRUE);

-- Sampled metrics snapshots (hypertable)
CREATE TABLE IF NOT EXISTS metrics (
    id              BIGSERIAL,
    server_id       INTEGER     NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    cpu_usage       DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_usage    DOUBLE PRECISION NOT NULL DEFAULT 0,
    request_count   INTEGER     NOT NULL DEFAULT 0,
    success_rate    DOUBLE PRECISION NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SELECT create_hypertable('metrics', 'created_at', if_not_exists => TRUE);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_requests_server_id_created ON requests (server_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_server_id_created  ON metrics  (server_id, created_at DESC);

-- Continuous aggregate: 1-minute averages (TimescaleDB feature)
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_1m
WITH (timescaledb.continuous) AS
SELECT
    server_id,
    time_bucket('1 minute', created_at) AS bucket,
    AVG(cpu_usage)    AS avg_cpu,
    AVG(memory_usage) AS avg_memory,
    SUM(request_count) AS total_requests,
    AVG(success_rate) AS avg_success_rate
FROM metrics
GROUP BY server_id, bucket;
