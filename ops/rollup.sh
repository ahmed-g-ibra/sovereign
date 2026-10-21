#!/usr/bin/env bash
# Recompute rollups. Safe to run as often as you like: buckets are recomputed,
# never accumulated, so overlapping runs converge on the same answer.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
export DATABASE_URL="${DATABASE_URL:-postgresql://sovereign:sovereign@localhost:55432/sovereign}"
cd "$ROOT/api"
uv run sovereign rollup --lookback 120
uv run sovereign partitions --ahead 7
