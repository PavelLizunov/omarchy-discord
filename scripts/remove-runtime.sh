#!/usr/bin/env bash
set -euo pipefail

purge=0
if [[ ${1:-} == "--purge" ]]; then
  purge=1
  shift
fi
if (( $# > 0 )); then
  echo "Usage: scripts/remove-runtime.sh [--purge]" >&2
  exit 2
fi

config_root=${XDG_CONFIG_HOME:-"$HOME/.config"}
cache_root=${XDG_CACHE_HOME:-"$HOME/.cache"}
unit_file="$config_root/systemd/user/omarchy-discord.service"
config_dir="$config_root/omarchy-discord"
cache_dir="$cache_root/omarchy-discord"
runtime_dir=${OMARCHY_DISCORD_RUNTIME_DIR:-"$HOME/.local/lib/omarchy-discord"}
backend_binary="$runtime_dir/omarchy-discord-backend"
stamp_file="$runtime_dir/installed-version"

systemctl --user stop omarchy-discord.service 2>/dev/null || true
rm -f -- "$unit_file" "$backend_binary" "$stamp_file"
rmdir -- "$runtime_dir" 2>/dev/null || true
systemctl --user daemon-reload

if [[ -d $config_dir ]]; then
  if (( purge )); then
    [[ $config_dir == "$config_root/omarchy-discord" ]] || exit 3
    rm -rf -- "$config_dir"
    echo "Removed Discord plugin configuration."
  else
    backup="${config_dir}.bak.$(date -u +%Y%m%d%H%M%S)"
    mv -- "$config_dir" "$backup"
    echo "Moved configuration to: $backup"
  fi
fi

if (( purge )); then
  if [[ -d $cache_dir ]]; then
    [[ $cache_dir == "$cache_root/omarchy-discord" ]] || exit 3
    rm -rf -- "$cache_dir"
    echo "Removed Discord media cache and build output."
  fi
  if command -v secret-tool >/dev/null 2>&1; then
    for _ in {1..20}; do
      secret-tool clear service quickshell-discord kind user-token >/dev/null 2>&1 || break
    done
    echo "Cleared matching Omarchy Discord keyring entries."
  fi
fi

echo "Runtime integration removed."
