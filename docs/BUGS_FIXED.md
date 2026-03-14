# File: docs/BUGS_FIXED.md
# Bugs Fixed & Refactoring Decisions

This document records every issue found in the original codebase and exactly how it was resolved.

---

## Critical Bugs

### BUG-01 — ML response field mismatch
**File:** `backend/internal/loadbalancer/strategies_ml.go` vs `ml/model-server/main.go`

**Problem:** The Go load-balancer strategy expected the model server to return `{"predictions":[...]}` but the model server returned `{"scores":[...]}`. This made ML routing silently fall back to WeightedRoundRobin on every single request without any error message.

**Fix:** Both sides now use `{"predictions":[...]}`. The `mlResponse` struct in `ml_strategy.go` and the `predictResponse` struct in `ml/model-server/main.go` both use the field name `Predictions json:"predictions"`.

---

### BUG-02 — ONNX model input/output name mismatch
**File:** `ml/model-server/main.go`

**Problem:** The model server hard-coded TensorFlow SavedModel-style names (`serving_default_input:0`, `StatefulPartitionedCall:0`) but the training script used `torch.onnx.export` which produces PyTorch-style names (`features`, `predicted_score`). The ONNX session would fail to load the model entirely.

**Fix:** Input/output names are now read from `MODEL_INPUT_NAME` (default `features`) and `MODEL_OUTPUT_NAME` (default `predicted_score`) environment variables. This matches the training script defaults and allows overriding without a code change if the model is retrained differently.

---

### BUG-03 — Duplicate `InsertRequest` database write
**File:** `backend/internal/metrics/collector.go`

**Problem:** `RecordRequest` called `database.InsertRequest` twice — once at step 4 and again at step 8 of the same function. Every request created two records in the `requests` table, doubling storage usage and corrupting success-rate calculations.

**Fix:** Removed the duplicate call. `InsertRequest` is called exactly once per request in the new `collector.go`.

---

### BUG-04 — Synchronous HTTP fetch on the request path
**File:** `backend/internal/metrics/collector.go`

**Problem:** `RecordRequest` made a live HTTP call to the backend server's `/metrics` endpoint to get CPU/memory. This blocked the calling goroutine for up to 2 seconds on every routed request, adding latency under load.

**Fix:** CPU/memory polling is moved to `runMetricsPolling()`, a background goroutine that polls every 10 seconds independently of request handling. `RecordRequest` only updates Prometheus counters — no network I/O.

---

### BUG-05 — SQL placeholder syntax wrong (PostgreSQL vs MySQL)
**File:** `backend/internal/api/handlers.go` — `RegisterServer` handler

**Problem:** The handler used `?` placeholders (MySQL/SQLite syntax) in a PostgreSQL query. This would panic at runtime with `pq: syntax error at or near "?"`.

**Fix:** The `RegisterServer` handler was removed entirely (server registration happens automatically at startup via `UpsertServer`). All remaining queries use `$1`, `$2`, etc.

---

### BUG-06 — Cache key collision in ML strategy
**File:** `backend/internal/loadbalancer/strategies_ml.go`

**Problem:** `generateCacheKey` concatenated values without separators: `fmt.Sprintf("%d%.2f%.2f", id, cpu, mem)`. Server ID `12` with CPU `3.00` produced the same key as server IDs `1` and `2` with CPU `3.00` (i.e. `"123.003.00"` vs `"123.003.00"`).

**Fix:** The key now uses pipe separators: `fmt.Fprintf(&b, "%d|%.2f|%.2f|", id, cpu, mem)`, making every field boundary unambiguous.

---

### BUG-07 — ML default endpoint port wrong
**File:** `backend/cmd/api/main.go`

**Problem:** The fallback `ML_MODEL_ENDPOINT` was `http://ml-service:8000` but the model server listens on `8081`. When `ML_MODEL_ENDPOINT` was not set, every ML prediction failed with a connection refused error.

**Fix:** The default is now `http://ml-service:8081`, matching the model server's `ML_SERVICE_PORT` default.

---

### BUG-08 — docker-compose ML service port mapping wrong
**File:** `docker-compose.yml`

**Problem:** The original `ml-model` service mapped `5000:5000` but the model server binds to port 8081. The container was unreachable from other services.

**Fix:** Port mapping updated to `8081:8081`. The `ML_SERVICE_PORT` env var is also passed explicitly so the binding is always consistent with the exposed port.

---

### BUG-09 — `CanHandleRequest` with default capacity=1 causes immediate fallback
**File:** `backend/internal/loadbalancer/strategies_ml.go`

**Problem:** `CanHandleRequest` allowed `Connections < Capacity * 2`. With the default `capacity = 1` from a missing DB row, a server could handle at most 1 connection before the ML strategy considered it at capacity and skipped it. Under any real load all servers would be "at capacity" and ML would always fall back.

**Fix:** Default capacity is set to `10` in `GetServerCapacity` (was `1`). The `CanAcceptRequest` logic on `Server` is also more conservative, checking `Connections < Capacity` (not `2×`) — this is accurate. For genuine over-provisioning, raise the capacity value in the DB.

---

## Dead Code Removed

| File | Reason |
|---|---|
| `backend/cmd/api/server.go` | `startBackendServer()` defined but never called. Backend servers moved to `backend/cmd/server/main.go`. |
| `backend/internal/metrics/storage.go` | Contained `var DB *sql.DB` that was never assigned. All functions would nil-panic. Functionality replaced by `database.DB` methods. |
| `backend/cmd/server5000/main.go`, `server5001/main.go`, `server5002/main.go` | Three identical files differing only in hardcoded port. Replaced by single `backend/cmd/server/main.go` with `BACKEND_PORT` env var. |

---

## Hardcoded Values Eliminated

| Original | Replacement |
|---|---|
| `"http://localhost:5000"` etc. | `SERVERS` env var |
| `":8080"` in main | `APP_PORT` env var |
| `"http://ml-service:8000"` | `ML_MODEL_ENDPOINT` env var |
| `8081` in model server | `ML_SERVICE_PORT` env var |
| `ml/models/load_balancer.onnx` | `MODEL_PATH` env var |
| `ml/models/scaler.json` | `SCALER_PATH` env var |
| ONNX DLL path `C:/Users/souvi/...` | `ONNX_LIB_PATH` env var with per-OS default |
| `"serving_default_input:0"` | `MODEL_INPUT_NAME` env var |
| DB credentials in all files | `DB_*` env vars via central config |
| `30` s circuit breaker reset | `ML_CIRCUIT_BREAKER_RESET_SECONDS` env var |
| `1000` LRU cache size | `ML_CACHE_SIZE` env var |
| `5s` health check interval | `HEALTH_CHECK_INTERVAL_SECONDS` env var |

---

## Architecture Improvements

| Area | Before | After |
|---|---|---|
| Config | Scattered `os.Getenv` calls across 10+ files | Single `config.Load()` with validation, typed struct, and documented defaults |
| Logging | Mix of `log.Println` and no structured fields | `zap` everywhere, JSON format for Loki ingestion |
| Tracing | Missing | OpenTelemetry → OTel Collector → Tempo, linked to Loki logs |
| Middleware | Duplicate registration in `main.go` and `router.go` | Single registration point in `router.go` |
| Routes | Duplicate `/metrics` route | Single registration via `promhttp.Handler()` |
| DB layer | Direct `database.DB` package-level var | Injected `*database.DB` with pool config from env |
| Metrics polling | Blocking on request path | Background goroutine, decoupled from request latency |
| Circuit breaker | Hardcoded 30 s reset | Config-driven `ML_CIRCUIT_BREAKER_RESET_SECONDS` |
| Docker images | Alpine with shell — larger, more attack surface | `distroless/static` (balancer + backend), non-root user |
| Health probes | Single `/health` for both liveness and readiness | Separate `/health/live` and `/health/ready` for Kubernetes |
| Graceful shutdown | 10 s timeout for LB, 5 s for backend | 30 s for LB (allows long-lived connections), 10 s for backend |
