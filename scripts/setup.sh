#!/usr/bin/env bash
set -euo pipefail

source_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
skip_backend_build=${OMARCHY_DISCORD_SKIP_BACKEND_BUILD:-0}
force_reinstall=0

usage() {
  cat <<'USAGE'
Usage: scripts/setup.sh [--skip-backend-build] [--reinstall-backend]

Install the Discord backend into ~/.local/lib/omarchy-discord/ and its static
user unit into ~/.config/systemd/user/. The unit is never enabled at login.
USAGE
}

while (( $# > 0 )); do
  case $1 in
    --skip-backend-build) skip_backend_build=1; shift ;;
    --reinstall-backend) force_reinstall=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "setup.sh: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

for command_name in secret-tool systemctl install; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "setup.sh: required Omarchy base command is missing: $command_name" >&2
    exit 1
  }
done

config_root=${XDG_CONFIG_HOME:-"$HOME/.config"}
unit_dir="$config_root/systemd/user"
unit_name=omarchy-discord.service
unit_file="$unit_dir/$unit_name"
runtime_dir=${OMARCHY_DISCORD_RUNTIME_DIR:-"$HOME/.local/lib/omarchy-discord"}
backend_binary="$runtime_dir/omarchy-discord-backend"
stamp_file="$runtime_dir/installed-version"
plugin_version=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
  -- "$source_root/manifest.json" 2>/dev/null | head -n 1)

backend_stale() {
  [[ -x $backend_binary ]] || return 0
  local newer
  newer=$(find "$source_root/backend" -type f -newer "$backend_binary" \
    -not -path '*/testdata/*' -not -path '*/target/*' -print -quit 2>/dev/null)
  [[ -n $newer ]]
}

backend_ready=0
build_status=0
if [[ -x $backend_binary && $force_reinstall -eq 0 ]] && ! backend_stale; then
  backend_ready=1
elif (( skip_backend_build )); then
  if [[ -x $backend_binary ]]; then
    echo "setup.sh: backend sources are newer than $backend_binary; keeping it (--skip-backend-build)" >&2
    backend_ready=1
  else
    build_status=30
  fi
else
  "$source_root/scripts/build-backend.sh" || build_status=$?
  if (( build_status == 0 )); then
    backend_ready=1
  fi
fi

if (( ! backend_ready )); then
  if (( build_status == 32 )); then
    exit 32
  fi
  if (( build_status == 30 || build_status == 0 )); then
    echo "setup.sh: no Discord backend ships for $(uname -m) and Go is not installed" >&2
    echo "Install Go to build it, or use a release that ships backend/dist/$(uname -m)/." >&2
    exit 30
  fi
  echo "setup.sh: the Discord backend could not be built; the output above has the details" >&2
  exit 31
fi

if ldd "$backend_binary" 2>/dev/null | grep -q 'not found'; then
  echo "setup.sh: the Discord backend needs libopus and it is not installed" >&2
  echo "Install it and re-run: sudo pacman -S opus" >&2
  exit 32
fi

install -d -m 700 -- "$unit_dir"
if [[ -n ${OMARCHY_DISCORD_RUNTIME_DIR:-} ]]; then
  sed -e "s|^ExecStart=.*|ExecStart=$backend_binary|" \
    -- "$source_root/systemd/$unit_name" > "$unit_file.tmp"
  install -m 644 -- "$unit_file.tmp" "$unit_file"
  rm -f -- "$unit_file.tmp"
else
  install -m 644 -- "$source_root/systemd/$unit_name" "$unit_file"
fi
systemctl --user daemon-reload

unit_state=$(systemctl --user is-enabled "$unit_name" 2>/dev/null || true)
if [[ $unit_state == "enabled" || $unit_state == "enabled-runtime" ]]; then
  echo "Warning: $unit_name was already enabled at login." >&2
  echo "For on-demand behavior, run: systemctl --user disable $unit_name" >&2
fi

printf '%s\n' "${plugin_version:-unknown}" > "$stamp_file"
chmod 600 -- "$stamp_file"

echo "Installed backend: $backend_binary"
echo "Installed user unit: $unit_file"
echo "The backend stays stopped until the plugin starts it."
