#!/usr/bin/env bash
# Install the backend binary and its static user unit. Unprivileged; never
# starts or enables anything.
#
# Exit codes (propagated from build-backend.sh, and read by DaemonManager.qml
# through scripts/backend-runtime.sh sync):
#   0   the backend and the unit are installed
#   30  no prebuilt for this architecture and no Go toolchain to build one
#   31  the backend failed to build or install
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
# Records the plugin version this runtime was installed from; its mtime is the
# marker scripts/backend-runtime.sh sync compares the shipped backend against.
stamp_file="$runtime_dir/installed-version"
plugin_version=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
  -- "$source_root/manifest.json" 2>/dev/null | head -n 1)

# True when any backend source (or a bundled prebuilt) is newer than the
# installed binary, so re-running setup picks up upgrades.
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
  if (( build_status == 30 || build_status == 0 )); then
    echo "setup.sh: no Discord backend ships for $(uname -m) and Go is not installed" >&2
    echo "Install Go to build it, or use a release that ships backend/dist/$(uname -m)/." >&2
    exit 30
  fi
  echo "setup.sh: the Discord backend could not be built; the output above has the details" >&2
  exit 31
fi

install -d -m 700 -- "$unit_dir"
if [[ -n ${OMARCHY_DISCORD_RUNTIME_DIR:-} ]]; then
  # The repo unit hard-codes the default %h path; point the installed copy at
  # the custom runtime dir so ExecStart matches where the binary went.
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

# Written last, so a failed install never leaves a stamp claiming success. Its
# mtime is what marks this runtime as current for the shipped backend.
printf '%s\n' "${plugin_version:-unknown}" > "$stamp_file"
chmod 600 -- "$stamp_file"

echo "Installed backend: $backend_binary"
echo "Installed user unit: $unit_file"
echo "The backend stays stopped until the plugin starts it."
