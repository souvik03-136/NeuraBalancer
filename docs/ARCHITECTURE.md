# File: docs/ARCHITECTURE.md
# NeuraBalancer — Architecture Reference

## System Components

### Load Balancer (`backend/cmd/api`)

The core binary. It:

1. Loads all configuration from environment at startup — no config files read at runtime.
2. Opens a PostgreSQL connection pool (TimescaleDB) and runs schema migrations on startup.
3. Instantiates a Prometheus metrics collector (singleton) and an OpenTelemetry tracer.
4. Builds the chosen strategy from `LB_STRATEGY` and injects it into the `LoadBalancer`.
5. Starts two background goroutines: health checker (every `HEALTH_CHECK_INTERVAL_SECONDS`) and metrics poller (every 10 s).
6. Serves the Echo HTTP router. Every request is traced (OTLP → Tempo) and logged (JSON → Loki via Promtail).
7. On `SIGTERM`/`SIGINT`, drains in-flight requests within 30 s then exits cleanly.

### Backend Servers (`backend/cmd/server`)

A minimal HTTP server that simulates application workloads. Configured entirely by environment:

- `BACKEND_PORT` — which port to bind (default `8001`)
- `BACKEND_INSTANCE_ID` — used in log and response fields

Exposes:
- `GET /health` → `{"status":"ok","instance_id":"..."}`
- `GET /metrics` → `{"cpu_usage":X,"memory_usage":Y}` — polled by the LB metrics collector
- `GET /` → echo of instance info

CPU and memory are sampled via `gopsutil` every 2 s with exponential smoothing (`α=0.3`) to reduce noise.

### ML Model Server (`ml/model-server`)

A Go HTTP server wrapping the ONNX Runtime C library via `yalue/onnxruntime_go`. Configured entirely by environment:

- `ML_SERVICE_PORT` — HTTP port (default `8081`)
- `MODEL_PATH` — path to `.onnx` file
- `SCALER_PATH` — path to `scaler.json`
- `MODEL_INPUT_NAME` / `MODEL_OUTPUT_NAME` — ONNX graph node names
- `ONNX_LIB_PATH` — path to the `.so`/`.dll`

On `POST /predict`, the server:
1. Reads a JSON array of server feature vectors.
2. Normalises each using the StandardScaler parameters from `scaler.json`.
3. Runs ONNX inference (thread-safe via `sync.RWMutex`).
4. Returns a `{"predictions":[f32,...]}` array — one score per server.

### Strategy Selection

```
LB_STRATEGY=ml  ──► MLStrategy
                        │ circuit breaker open?
                        ├─ YES ──► WeightedRoundRobin (fallback)
                        └─ NO
                             │ cache hit?
                             ├─ YES ──► use cached predictions
                             └─ NO
                                  │ call /predict (timeout = ML_MODEL_TIMEOUT_MS)
                                  ├─ error ──► trip circuit breaker ──► WRR
                                  └─ success ──► cache + select lowest score
```

The circuit breaker auto-resets after `ML_CIRCUIT_BREAKER_RESET_SECONDS`. The LRU cache holds up to `ML_CACHE_SIZE` entries keyed by `serverID|cpu|memory|` (pipe-separated to prevent collision).

## Data Flow

```
Client Request
    │
    ▼
Echo Router  ─── RequestID middleware injects X-Request-Id
    │        ─── OTel middleware starts trace span
    │        ─── StructuredLogger records request fields
    │
    ▼
Handler.ProxyRequest
    │
    ▼
LoadBalancer.NextServer  ─── Strategy.Select(healthy servers)
    │                    ─── increments server.Connections
    │
    ▼
http.DefaultClient.Do   ─── forwards to backend URL
    │
    ▼
Response copied to client
    │
    ▼
Collector.RecordRequest  ─── Prometheus counters/histograms updated
                         ─── DB write (goroutine, non-blocking)
```

## Database Schema

```
servers       – registry of known backends (upserted on startup)
requests      – per-request log (TimescaleDB hypertable, partitioned by time)
metrics       – sampled CPU/memory snapshots (TimescaleDB hypertable)
metrics_1m    – continuous aggregate: 1-minute roll-ups of metrics
```

The `requests` and `metrics` tables are TimescaleDB hypertables. Queries over time ranges (e.g., success rate for the last 5 minutes) use the time-based partition index and are fast regardless of total row count.

## Observability Pipeline

```
Go app (zap JSON logs)
    │
    ▼ Docker log driver (json-file with container name tag)
Promtail  ─── reads /var/lib/docker/containers/**/*.log
    │     ─── relabels: job=container-name, service=compose-service
    │     ─── pipeline: parse JSON, extract level/request_id/duration
    ▼
Loki  ─── stores log streams, indexed by service + level

Go app (Prometheus metrics at /metrics)
    │
Prometheus  ─── scrapes every 15 s
    │
Grafana  ─── queries Prometheus + Loki + Tempo

Go app (OTLP gRPC traces)
    │
OTel Collector  ─── batches + sanitises (strips auth headers)
    │
Tempo  ─── stores traces, queryable via Grafana
```

Grafana links traces to logs via the `traceID` derived field in the Loki datasource configuration. Click a trace in Grafana → Tempo, then jump directly to the correlated log lines.

## Scaling Considerations

### Horizontal scaling of the load balancer

The load balancer is stateless — session state lives in TimescaleDB. Deploy multiple replicas behind an external load balancer (e.g., AWS ALB, NGINX, or a Kubernetes Service of type `LoadBalancer`). Each LB instance independently health-checks backends and makes routing decisions.

### Adding more backend servers

1. Deploy a new backend container with a unique `BACKEND_PORT` and `BACKEND_INSTANCE_ID`.
2. Add its URL to `SERVERS`.
3. Restart the load balancer (or use Kubernetes rolling update). The new server is registered in the DB and enters the rotation immediately.

No code changes required. The ML model will incorporate it as soon as it accumulates enough request history to be included in retraining.

### ML model scaling

The ONNX model server is CPU-bound during inference. Deploy 2+ replicas behind an internal service for production. Prediction latency with the default model is under 1 ms per server on modern hardware; the 300 ms timeout is very conservative.
