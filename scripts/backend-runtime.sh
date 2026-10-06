#!/usr/bin/env bash
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

runtime_current() {
  [[ -x $backend_binary && -f $unit_file && -f $stamp_file ]] || return 1
  [[ $(cat -- "$stamp_file" 2>/dev/null) == "$(plugin_version)" ]] || return 1
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
    load_state=$(systemctl --user show --property=LoadState --value "$backend_unit")
    if [[ $load_state != "not-found" ]]; then
      systemctl --user stop "$backend_unit"
      active_state=$(systemctl --user show --property=ActiveState --value "$backend_unit")
      case $active_state in
        inactive|failed) ;;
        *) echo "backend-runtime.sh: backend is not stopped ($active_state)" >&2; exit 1 ;;
      esac
    fi
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
