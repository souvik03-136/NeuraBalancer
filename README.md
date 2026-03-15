# NeuraBalancer

**An AI-driven, self-optimising HTTP load balancer** — routes requests using real-time server metrics and an ONNX reinforcement-learning model, with full observability (Prometheus · Grafana · Loki · Tempo).

> **Want to see it running?**  
> The complete end-to-end demo — terminal output, Prometheus graphs, Grafana dashboards, Loki logs, and database verification — is documented in **[DEMO_WALKTHROUGH.md](./DEMO_WALKTHROUGH.md)**.

---

## Table of Contents

1. [Architecture](#architecture)
2. [Features](#features)
3. [Project Structure](#project-structure)
4. [Prerequisites](#prerequisites)
5. [Quick Start](#quick-start)
6. [Configuration Reference](#configuration-reference)
7. [Load Balancing Strategies](#load-balancing-strategies)
8. [API Reference](#api-reference)
9. [Observability Stack](#observability-stack)
10. [ML Model](#ml-model)
11. [Development Guide](#development-guide)
12. [Testing](#testing)
13. [Deployment](#deployment)
14. [Contributing](#contributing)

---

## Architecture

```
Clients
  │
  ▼
Load Balancer  :8080   (Go · Echo · Prometheus metrics)
  ├─► Backend-1 :8001  ◄──┐
  ├─► Backend-2 :8002     │  Health checks every 5 s
  └─► Backend-3 :8003  ◄──┘
  │
  ├─► ML Service    :8081   (Go · ONNX Runtime)
  ├─► TimescaleDB   :5432   (request + metrics storage)
  └─► Observability
        ├── Prometheus  :9090
        ├── Grafana     :3000
        ├── Loki        :3100  (log aggregation)
        ├── Tempo       :3200  (distributed traces)
        └── OTel Coll.  :4317  (OTLP receiver)
```

All configuration is **environment-driven** — no hardcoded values anywhere in the codebase.

---

## Features

| Category | Detail |
|---|---|
| **Strategies** | Round Robin, Weighted Round Robin, Least Connections, Random, ML |
| **ML Routing** | ONNX inference, LRU prediction cache, circuit breaker, WRR fallback |
| **Resilience** | Active health checks, automatic server recovery, graceful shutdown |
| **Observability** | Prometheus metrics, structured JSON logs → Loki, OTLP traces → Tempo → Grafana |
| **Scalability** | Stateless LB; add backend instances by extending `SERVERS` env var |
| **Security** | Distroless images, non-root containers, no secrets in images |
| **Dev Experience** | Single `task up` start, pre-commit hooks, golangci-lint, pytest, coverage |

---

## Project Structure

```
neurabalancer/
│
├── backend/
│   ├── cmd/
│   │   ├── api/           # Load balancer entrypoint
│   │   └── server/        # Generic backend server (config-driven port)
│   ├── internal/
│   │   ├── api/           # Echo handlers, middleware, router
│   │   ├── config/        # Config loader + zap logger factory
│   │   ├── database/      # PostgreSQL connection + all queries
│   │   ├── loadbalancer/  # Balancer core, strategies, ML strategy
│   │   ├── metrics/       # Prometheus collector
│   │   └── tracer/        # OpenTelemetry setup
│   └── migrations/        # SQL schema migrations
│
├── ml/
│   ├── model-server/      # ONNX inference HTTP server
│   ├── models/            # .onnx + scaler.json (git-ignored, generated)
│   ├── scripts/           # deploy_model.sh
│   └── training/          # PyTorch training pipeline + tests
│
├── configs/
│   ├── prometheus/        # prometheus.yml
│   ├── loki/              # loki.yml + promtail.yml
│   ├── tempo/             # tempo.yml
│   ├── otel/              # otel-collector.yml
│   └── grafana/           # Provisioned datasources + dashboards
│
├── deployments/
│   ├── docker/            # Dockerfile.balancer / .backend / .ml
│   ├── helm/              # Helm chart for Kubernetes
│   └── k8s/               # Raw Kubernetes manifests
│
├── scripts/               # healthcheck.sh, wait-for.sh
├── .github/workflows/     # CI (test + lint) and CD (build + deploy)
├── docker-compose.yml     # Full local stack
├── Taskfile.yml           # Developer task runner
├── .env.example           # All available configuration keys
├── .golangci.yml          # Linter configuration
└── DEMO_WALKTHROUGH.md    # End-to-end demo with screenshots
```

---

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Docker + Compose | 24+ / v2+ | Running the full stack |
| Go | 1.21+ | Building Go services |
| Python | 3.11+ | ML training |
| Task | 3+ | Task runner (`brew install go-task`) |
| golangci-lint | 1.56+ | Go linting |

---

## Quick Start

### 1 — Clone and configure

```bash
git clone https://github.com/souvik03-136/neurabalancer.git
cd neurabalancer
cp .env.example .env
```

Open `.env` and set **at minimum**:

```dotenv
DB_PASSWORD=your_secure_password
GRAFANA_ADMIN_PASSWORD=your_grafana_password
```

### 2 — Start the full stack

```bash
task up
```

This builds all images and starts every service. First run takes ~3 minutes.

### 3 — Verify everything is healthy

```bash
task ps
# All containers should show (healthy)

curl http://localhost:8080/health/live
# {"status":"ok","ts":"..."}

curl http://localhost:8080/health/ready
# {"status":"ready","total":3,"healthy":3}

curl http://localhost:8080/api/v1/servers
# Lists all backend servers and their state
```

### 4 — Open the dashboards

| Service | URL | Credentials |
|---|---|---|
| Grafana | http://localhost:3000 | `admin` / `$GRAFANA_ADMIN_PASSWORD` |
| Prometheus | http://localhost:9090 | — |
| Load Balancer metrics | http://localhost:8080/metrics | — |

The **NeuraBalancer — Overview** dashboard is pre-provisioned in Grafana.

---

## Demo Walkthrough

If you want to verify the system is working end-to-end — or show it to someone — the **[DEMO_WALKTHROUGH.md](./DEMO_WALKTHROUGH.md)** covers everything in order:

| Phase | What it covers |
|---|---|
| [Phase 1](./DEMO_WALKTHROUGH.md#phase-1--stack-startup) | All 13 containers healthy, load balancer startup logs, backend server logs |
| [Phase 2](./DEMO_WALKTHROUGH.md#phase-2--api-health-checks) | Liveness probe, readiness probe, server inventory |
| [Phase 3](./DEMO_WALKTHROUGH.md#phase-3--sending-real-traffic) | Routing individual requests and 100-request burst |
| [Phase 4](./DEMO_WALKTHROUGH.md#phase-4--raw-prometheus-metrics) | Raw `neurabalancer_*` metrics from `/metrics` endpoint |
| [Phase 5](./DEMO_WALKTHROUGH.md#phase-5--database-verification) | TimescaleDB tables — servers registered, requests recorded |
| [Phase 6](./DEMO_WALKTHROUGH.md#phase-6--prometheus-ui) | Prometheus UI — targets, request rate, P95 latency, CPU graphs |
| [Phase 7](./DEMO_WALKTHROUGH.md#phase-7--grafana-dashboards--loki-logs) | Grafana overview dashboard, Loki structured log explorer |
| [Phase 8](./DEMO_WALKTHROUGH.md#phase-8--ml-service-status) | ML service degraded mode — expected on fresh install |

---

## Configuration Reference

All configuration is loaded from environment variables. Copy `.env.example` to `.env` to get started. Every key has a documented default — see `.env.example` for the full reference.

### Core variables

| Variable | Default | Description |
|---|---|---|
| `APP_ENV` | `development` | Environment name (`development`, `staging`, `production`) |
| `APP_PORT` | `8080` | Load balancer HTTP port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `json` for structured logs; `text` for human-readable |
| `LB_STRATEGY` | `least_connections` | Load balancing algorithm |
| `SERVERS` | _(required)_ | Comma-separated backend URLs |

### Database

| Variable | Default | Description |
|---|---|---|
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_NAME` | `neurabalancer` | Database name |
| `DB_USER` | `postgres` | Database user |
| `DB_PASSWORD` | _(required)_ | Database password |
| `DB_SSLMODE` | `disable` | Use `require` in production |

### ML Service

| Variable | Default | Description |
|---|---|---|
| `ML_MODEL_ENDPOINT` | `http://ml-service:8081` | ONNX model server URL |
| `ML_MODEL_TIMEOUT_MS` | `300` | Per-prediction timeout |
| `ML_CIRCUIT_BREAKER_RESET_SECONDS` | `30` | Seconds before circuit breaker auto-resets |
| `ML_CACHE_SIZE` | `1000` | LRU prediction cache entries |

---

## Load Balancing Strategies

Select a strategy by setting `LB_STRATEGY` in `.env`:

| Strategy | Value | Description |
|---|---|---|
| Least Connections | `least_connections` | Routes to the server with the fewest active requests. Best general-purpose default. |
| Round Robin | `round_robin` | Cycles through servers in sequence. Fastest algorithm. |
| Weighted Round Robin | `weighted_round_robin` | Like round robin but honours per-server `weight` values set in the DB or via `SERVER_WEIGHT_*` env vars. |
| Random | `random` | Selects a server uniformly at random. |
| ML | `ml` | Uses ONNX model to predict the lowest-latency server. Falls back to Weighted Round Robin if the model service is unavailable or the circuit breaker is open. |

### Changing strategy at runtime

Restart only the load-balancer container — no data loss:

```bash
# Edit .env: LB_STRATEGY=ml
docker compose up -d --no-deps load-balancer
```

---

## API Reference

| Method | Path | Description |
|---|---|---|
| `GET` | `/health/live` | Liveness probe — always 200 if process is up |
| `GET` | `/health/ready` | Readiness probe — 503 if no healthy backends |
| `GET` | `/metrics` | Prometheus metrics scrape endpoint |
| `GET` | `/api/v1/servers` | List all backend servers and their state |
| `ANY` | `/api/v1/request` | Proxy a request to the best backend |
| `ANY` | `/api/v1/request/*` | Proxy with arbitrary path suffix |

### Example: Route a request

```bash
curl -X POST http://localhost:8080/api/v1/request \
  -H "Content-Type: application/json" \
  -d '{"key": "value"}'
```

---

## Observability Stack

### Prometheus metrics

All metrics are prefixed `neurabalancer_`:

| Metric | Type | Description |
|---|---|---|
| `http_requests_total` | Counter | Requests by method, path, status, server_id |
| `http_request_duration_seconds` | Histogram | Request latency |
| `server_cpu_usage_percent` | Gauge | Backend CPU % |
| `server_memory_usage_percent` | Gauge | Backend memory % |
| `server_active_connections` | Gauge | In-flight connections |
| `server_error_rate` | Gauge | Rolling error rate |
| `ml_predictions_total` | Counter | Successful ML predictions |
| `ml_errors_total` | Counter | ML errors (fallback triggers) |
| `ml_cache_hits_total` | Counter | LRU cache hits |
| `ml_circuit_breaker_open` | Gauge | 1 = circuit open, 0 = closed |
| `ml_inference_duration_seconds` | Histogram | ONNX inference latency |

### Structured logs (Loki)

Every request produces a JSON log line containing:
`request_id`, `method`, `path`, `status`, `duration`, `remote_ip`, `server_id`

Query in Grafana:
```logql
{service="load-balancer"} | json | status >= 500
{service="load-balancer"} | json | duration > 1s
```

### Distributed traces (Tempo)

Every request carries an OpenTelemetry trace. View in Grafana → Explore → Tempo. Traces link to logs via `traceID` derived field in Loki.

---

## ML Model

### How it works

1. **Feature collection**: For each healthy server, the ML strategy gathers 6 features: `cpu_usage`, `memory_usage`, `active_conns`, `error_rate`, `response_p95`, `capacity`.
2. **Inference**: Features are sent to the ONNX model server. The server normalises them using `scaler.json` and runs inference.
3. **Selection**: The server with the lowest predicted score (= expected latency × load) is selected, subject to capacity constraints.
4. **Fallback**: If inference fails or the circuit breaker is open, the strategy falls back to Weighted Round Robin automatically.

### Training a new model

The system must have collected at least a few hours of request data before training.

```bash
# Ensure DB has data, then:
task ml-train
# Outputs: ml/models/load_balancer.onnx, scaler.json, inference_features.json

# Validate and hot-reload the model server:
bash ml/scripts/deploy_model.sh
```

### ONNX input/output names

The model server reads `MODEL_INPUT_NAME` and `MODEL_OUTPUT_NAME` from environment:

```dotenv
MODEL_INPUT_NAME=features           # matches torch.onnx.export input_names
MODEL_OUTPUT_NAME=predicted_score   # matches torch.onnx.export output_names
```

These default to the values the training script uses. Only change them if you retrain with a different export configuration.

---

## Development Guide

### Running services individually

```bash
# Start only infrastructure (DB, Redis, observability)
docker compose up -d postgres redis prometheus grafana loki tempo otel-collector

# Run load balancer locally
task build-balancer
DB_HOST=localhost SERVERS=http://localhost:8001 ./bin/neurabalancer

# Run a backend server locally (port from env)
BACKEND_PORT=8001 BACKEND_INSTANCE_ID=local-1 ./bin/backend-server
```

### Adding a new backend server

1. Add a new service in `docker-compose.yml` using the same pattern as `backend-1`.
2. Append its URL to the `SERVERS` env var in `docker-compose.yml`.
3. Run `docker compose up -d`.

No code changes required.

### Adding a new load-balancing strategy

1. Create a struct implementing the `Strategy` interface in `backend/internal/loadbalancer/`.
2. Register it in `backend/internal/loadbalancer/factory.go`.
3. Add the strategy name to the config validation in `backend/internal/config/config.go`.
4. Add tests in `strategies_test.go`.

---

## Testing

```bash
# All Go tests with race detector
task test

# Tests + HTML coverage report
task test-coverage

# Python feature-alignment tests
task test-python

# Run a quick load test (requires 'hey')
task load-test

# Lint
task lint
```

---

## Deployment

### Docker Compose (recommended for single-node)

```bash
cp .env.example .env   # Fill in production values
task up
```

### Kubernetes (Helm)

```bash
# Install
helm install neurabalancer deployments/helm/charts/neurabalancer \
  --set image.tag=v1.0.0 \
  --set db.password=<password> \
  --set grafana.adminPassword=<password>

# Upgrade
helm upgrade neurabalancer deployments/helm/charts/neurabalancer \
  --set image.tag=v1.1.0

# Status
kubectl get pods -l app.kubernetes.io/name=neurabalancer
```

### CI/CD

- **CI** runs on every pull request and push to `main`/`develop` — linting, tests, Docker build check.
- **CD** runs on version tags (`v*.*.*`) — builds and pushes images to GHCR, then syncs ArgoCD.

Set these repository secrets for CD:
- `ARGOCD_SERVER` — your ArgoCD server URL
- `ARGOCD_AUTH_TOKEN` — ArgoCD authentication token

---

## Contributing

1. Fork the repository
2. Run `task setup` to install hooks and dependencies
3. Create a feature branch: `git checkout -b feat/my-feature`
4. Commit using [Conventional Commits](https://www.conventionalcommits.org/): `feat:`, `fix:`, `chore:`, etc.
5. Push and open a pull request against `main`

Please keep PRs focused and small. Include tests for new functionality.