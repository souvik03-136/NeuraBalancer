#!/bin/sh
# File: scripts/wait-for.sh
# Polls TCP host:port until it accepts a connection or timeout expires.
# Usage:  ./scripts/wait-for.sh postgres:5432 -- echo "DB ready"
set -e

TIMEOUT="${WAIT_TIMEOUT:-60}"
WAIT_HOST="$(echo "$1" | cut -d: -f1)"
WAIT_PORT="$(echo "$1" | cut -d: -f2)"
shift

CMD="$*"

echo "Waiting up to ${TIMEOUT}s for ${WAIT_HOST}:${WAIT_PORT}..."

elapsed=0
until nc -z "$WAIT_HOST" "$WAIT_PORT" >/dev/null 2>&1; do
  if [ "$elapsed" -ge "$TIMEOUT" ]; then
    echo "Timeout waiting for ${WAIT_HOST}:${WAIT_PORT}" >&2
    exit 1
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done

echo "${WAIT_HOST}:${WAIT_PORT} is available after ${elapsed}s"
if [ -n "$CMD" ]; then
  exec $CMD
fi
