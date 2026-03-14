-- File: backend/migrations/001_initial_schema.down.sql
DROP MATERIALIZED VIEW  IF EXISTS metrics_1m;
DROP TABLE IF EXISTS metrics  CASCADE;
DROP TABLE IF EXISTS requests CASCADE;
DROP TABLE IF EXISTS servers  CASCADE;
