#!/usr/bin/env bash
# The only place the plugin's QML touches systemctl.
#
# Actions:
#   check   exit 0 when the installed binary and the user unit are both present
#   unit    print the unit name when the runtime is ready
#   status  exit 0 when the unit is active
#   start   start the unit
#   stop    stop the unit (unconditionally, never fails)
#   sync    install/refresh the runtime for the plugin version that just loaded
#
# sync exit codes (DaemonManager.qml maps these to UI messages):
#   0   the installed runtime already matches this plugin version (no work)
#   10  the runtime was installed or updated (and restarted if the binary moved)
#   30  no prebuilt for this architecture and no Go toolchain to build one
#   31  the backend failed to build or install
set -euo pipefail

action=${1:-}
if (( $# != 1 )); then
  echo "Usage: scripts/backend-runtime.sh check|unit|status|start|stop|sync" >&2
  exit 2
fi

source_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
config_root=${XDG_CONFIG_HOME:-"$HOME/.config"}
runtime_dir=${OMARCHY_DISCORD_RUNTIME_DIR:-"$HOME/.local/lib/omarchy-discord"}
backend_binary="$runtime_dir/omarchy-discord-backend"
stamp_file="$runtime_dir/installed-version"
backend_unit=omarchy-discord.service
unit_file="$config_root/systemd/user/$backend_unit"

unit_exists() {
  systemctl --user cat "$backend_unit" >/dev/null 2>&1
}

runtime_ready() {
  [[ -x $backend_binary ]] && unit_exists
}

plugin_version() {
  sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    -- "$source_root/manifest.json" 2>/dev/null | head -n 1
}

# Cheap enough to run on every plugin load: a few stats, one small read and one
# `find -quit`. No systemctl call, so a hot reload costs nothing.
runtime_current() {
  [[ -x $backend_binary && -f $unit_file && -f $stamp_file ]] || return 1
  [[ $(cat -- "$stamp_file" 2>/dev/null) == "$(plugin_version)" ]] || return 1
  # Anything the plugin ships under backend/ newer than the stamp means this
  # checkout moved on (a git-pulled update rewrites the files it changed).
  local newer
  newer=$(find "$source_root/backend" -type f -newer "$stamp_file" \
    -not -path '*/testdata/*' -not -path '*/target/*' -print -quit 2>/dev/null)
  [[ -z $newer ]]
}

binary_fingerprint() {
  [[ -f $backend_binary ]] || { printf 'missing\n'; return 0; }
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$backend_binary" | cut -d' ' -f1
  else
    stat -c '%s-%Y' -- "$backend_binary"
  fi
}

case $action in
  check)
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
  sync)
    was_ready=0
    if runtime_ready; then
      was_ready=1
    fi
    if (( was_ready )) && runtime_current; then
      exit 0
    fi
    before=$(binary_fingerprint)
    setup_status=0
    "$source_root/scripts/setup.sh" || setup_status=$?
    if (( setup_status != 0 )); then
      exit "$setup_status"
    fi
    after=$(binary_fingerprint)
    if [[ $before != "$after" ]]; then
      # Only restarts a unit that is already running, so an idle backend stays
      # stopped and a live session is handed the new binary immediately.
      systemctl --user try-restart "$backend_unit" >/dev/null 2>&1 || true
      exit 10
    fi
    if (( was_ready )); then
      exit 0
    fi
    exit 10
    ;;
  *)
    echo "backend-runtime.sh: unknown action: $action" >&2
    exit 2
    ;;
esac
