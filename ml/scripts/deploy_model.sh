#!/usr/bin/env bash
# File: ml/scripts/deploy_model.sh
# Trains a new model, validates it, and hot-reloads the ml-service container.
# All configuration comes from environment variables or .env.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$ROOT_DIR"

# Load .env if present
if [[ -f .env ]]; then
  set -a; source .env; set +a
fi

ML_SERVICE_CONTAINER="${ML_SERVICE_CONTAINER:-neurabalancer-ml-service-1}"

echo "==> [1/4] Training model..."
cd ml/training
python train_model.py
cd "$ROOT_DIR"

echo "==> [2/4] Validating feature alignment..."
pytest ml/training/tests/test_feature_alignment.py -v

echo "==> [3/4] Verifying model files exist..."
for f in ml/models/load_balancer.onnx ml/models/scaler.json ml/models/inference_features.json; do
  [[ -f "$f" ]] || { echo "ERROR: $f missing after training"; exit 1; }
done

echo "==> [4/4] Restarting ML service container..."
docker compose restart ml-service

echo "==> Waiting for health check..."
for i in $(seq 1 10); do
  if curl -sf "http://localhost:${ML_SERVICE_PORT:-8081}/health" >/dev/null 2>&1; then
    echo "==> ML service is healthy."
    exit 0
  fi
  echo "    Attempt $i/10 — waiting 3s..."
  sleep 3
done

echo "ERROR: ML service did not become healthy after restart."
exit 1
