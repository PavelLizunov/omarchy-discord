#!/usr/bin/env bash
# Install the backend binary and its static user unit. Unprivileged; never
# starts or enables anything. Exit 30 when no backend can be produced.
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

backend_ready=0
if [[ -x $backend_binary && $force_reinstall -eq 0 ]]; then
  backend_ready=1
elif (( ! skip_backend_build )); then
  if "$source_root/scripts/build-backend.sh"; then
    backend_ready=1
  fi
fi

if (( ! backend_ready )); then
  echo "setup.sh: the Discord backend is not available" >&2
  echo "Install Go to build it, or use a release that ships backend/dist/$(uname -m)/." >&2
  exit 30
fi

install -d -m 700 -- "$unit_dir"
install -m 644 -- "$source_root/systemd/$unit_name" "$unit_file"
systemctl --user daemon-reload

unit_state=$(systemctl --user is-enabled "$unit_name" 2>/dev/null || true)
if [[ $unit_state == "enabled" || $unit_state == "enabled-runtime" ]]; then
  echo "Warning: $unit_name was already enabled at login." >&2
  echo "For on-demand behavior, run: systemctl --user disable $unit_name" >&2
fi

echo "Installed backend: $backend_binary"
echo "Installed user unit: $unit_file"
echo "The backend stays stopped until the plugin starts it."
