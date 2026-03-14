#!/bin/sh
# File: scripts/healthcheck.sh
# Lightweight health check script used inside Docker containers.
# Falls back gracefully when wget is not available.
set -e

HOST="${HEALTH_HOST:-localhost}"
PORT="${APP_PORT:-8080}"
PATH_="${HEALTH_PATH:-/health/live}"

if command -v wget >/dev/null 2>&1; then
  wget -qO- "http://${HOST}:${PORT}${PATH_}" && exit 0
elif command -v curl >/dev/null 2>&1; then
  curl -sf "http://${HOST}:${PORT}${PATH_}" && exit 0
else
  echo "Neither wget nor curl found" >&2
  exit 1
fi
