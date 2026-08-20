#!/usr/bin/env bash
# The only place the plugin's QML touches systemctl.
set -euo pipefail

action=${1:-}
if (( $# != 1 )); then
  echo "Usage: scripts/backend-runtime.sh check|unit|status|start|stop" >&2
  exit 2
fi

runtime_dir=${OMARCHY_DISCORD_RUNTIME_DIR:-"$HOME/.local/lib/omarchy-discord"}
backend_binary="$runtime_dir/omarchy-discord-backend"
backend_unit=omarchy-discord.service

unit_exists() {
  systemctl --user cat "$backend_unit" >/dev/null 2>&1
}

runtime_ready() {
  [[ -x $backend_binary ]] && unit_exists
}

case $action in
  check)
    # exit 0 when the installed binary and the user unit are both present
    runtime_ready
    ;;
  unit)
    runtime_ready || exit 1
    printf '%s\n' "$backend_unit"
    ;;
  status)
    runtime_ready || exit 1
    systemctl --user is-active "$backend_unit"
    ;;
  start)
    runtime_ready || {
      echo "backend-runtime.sh: the Discord backend is not installed; run scripts/setup.sh" >&2
      exit 1
    }
    systemctl --user start "$backend_unit"
    ;;
  stop)
    systemctl --user stop "$backend_unit" 2>/dev/null || true
    ;;
  *)
    echo "backend-runtime.sh: unknown action: $action" >&2
    exit 2
    ;;
esac
