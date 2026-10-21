#!/usr/bin/env bash
# Install the local stack as launchd jobs so measurements keep accumulating
# across reboots. Entirely free: nothing here touches a cloud provider.
#
#   ops/install-macos.sh          install and start
#   ops/install-macos.sh --remove stop and uninstall
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTS="$HOME/Library/LaunchAgents"
JOBS=(dev.sovereign.stack dev.sovereign.probe dev.sovereign.rollup)

if [ "${1:-}" = "--remove" ]; then
  for job in "${JOBS[@]}"; do
    launchctl bootout "gui/$(id -u)/$job" 2>/dev/null || true
    rm -f "$AGENTS/$job.plist"
    echo "removed $job"
  done
  exit 0
fi

if [ ! -x "$ROOT/agent/bin/probe" ]; then
  echo "building the probe first..."
  (cd "$ROOT/agent" && go build -o bin/probe ./cmd/probe)
fi

if [ ! -f "$ROOT/agent/state/identity.json" ]; then
  echo "ERROR: no probe identity found." >&2
  echo "Start the stack and enrol first:" >&2
  echo "  make up && make serve        # in one terminal" >&2
  echo "  make token                   # copy the token" >&2
  echo "  cd agent && ./bin/probe -register -server http://127.0.0.1:8000 \\" >&2
  echo "      -token TOKEN -name my-probe -country OM -city Muscat -tier core" >&2
  exit 1
fi

mkdir -p "$AGENTS" "$ROOT/data/logs"

for job in "${JOBS[@]}"; do
  sed "s|__ROOT__|$ROOT|g" "$ROOT/ops/launchd/$job.plist.template" > "$AGENTS/$job.plist"
  launchctl bootout "gui/$(id -u)/$job" 2>/dev/null || true
  launchctl bootstrap "gui/$(id -u)" "$AGENTS/$job.plist"
  echo "installed $job"
done

echo
echo "Running. Logs in $ROOT/data/logs/"
echo "Check progress any time with: make stats"
