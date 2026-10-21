#!/usr/bin/env bash
# Bring up the whole local stack and stay in the foreground.
#
# Written for a process supervisor (launchd, systemd) rather than a human: it
# waits for Postgres, applies migrations, then execs the API so the supervisor
# tracks the right process.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
export DATABASE_URL="${DATABASE_URL:-postgresql://sovereign:sovereign@localhost:55432/sovereign}"
PORT="${API_PORT:-8000}"

cd "$ROOT"
docker compose up -d db

echo "waiting for postgres..."
for _ in $(seq 1 60); do
  if [ "$(docker inspect --format '{{.State.Health.Status}}' sovereign-db-1 2>/dev/null)" = "healthy" ]; then
    break
  fi
  sleep 2
done

cd "$ROOT/api"
uv run sovereign migrate
exec uv run uvicorn sovereign_api.app:create_app --factory \
  --host 127.0.0.1 --port "$PORT" --log-level info
