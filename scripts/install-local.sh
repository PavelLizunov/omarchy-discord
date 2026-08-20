#!/usr/bin/env bash
# Developer install: validate, set up the backend, copy this checkout into the
# Omarchy user-plugin directory, and enable the widget.
set -euo pipefail

source_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
plugin_id=quickshell.discord
section=left

usage() {
  cat <<'USAGE'
Usage: scripts/install-local.sh [--section left|center|right]

Validate this checkout, install the Discord backend and unit, copy the checkout
into ~/.config/omarchy/plugins/quickshell.discord/ (a copy, not a symlink, so
plugin validation passes and in-tree edits do not hot-reload the shell), then
enable the widget through Omarchy's command. Re-run after every change.
USAGE
}

while (( $# > 0 )); do
  case $1 in
    --section)
      [[ $# -ge 2 ]] || { echo "install-local.sh: --section requires a value" >&2; exit 2; }
      section=$2
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) echo "install-local.sh: unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[[ $section =~ ^(left|center|right)$ ]] || {
  echo "install-local.sh: section must be left, center, or right" >&2
  exit 2
}

for command_name in omarchy omarchy-shell rsync jq; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "install-local.sh: required command is missing: $command_name" >&2
    exit 1
  }
done

omarchy plugin validate "$source_root"
"$source_root/scripts/setup.sh"

plugins_root="${XDG_CONFIG_HOME:-"$HOME/.config"}/omarchy/plugins"
target="$plugins_root/$plugin_id"
install -d -m 700 -- "$plugins_root"

if [[ -L $target ]]; then
  echo "install-local.sh: $target is a symlink; remove it first (omarchy plugin validate refuses symlinks)" >&2
  exit 1
fi
if [[ -e $target && ! -d $target ]]; then
  echo "install-local.sh: refusing to replace non-directory path: $target" >&2
  exit 1
fi
if [[ -d $target && ! -f $target/manifest.json ]]; then
  echo "install-local.sh: $target exists but is not a plugin checkout; refusing to sync into it" >&2
  exit 1
fi

# Every write inside the plugins dir reloads the shell's plugin system; rsync
# only touches changed files, and --delete keeps the copy an exact mirror.
rsync -a --delete --exclude='.git' --exclude='.claude' --exclude='backend/target' \
  -- "$source_root/" "$target/"
echo "Synced plugin copy: $target"

omarchy-shell shell rescanPlugins >/dev/null
discovered=0
for (( attempt = 0; attempt < 40; attempt++ )); do
  if omarchy plugin list --json | jq -e --arg id "$plugin_id" 'any(.[]; .id == $id)' >/dev/null; then
    discovered=1
    break
  fi
  sleep 0.05
done
(( discovered )) || {
  echo "install-local.sh: Omarchy did not discover $plugin_id" >&2
  exit 1
}

if omarchy plugin list --json | jq -e --arg id "$plugin_id" 'any(.[]; .id == $id and .enabled == true)' >/dev/null; then
  echo "Plugin already enabled."
else
  omarchy plugin enable "$plugin_id" --section "$section"
fi
echo "Installed. Click the Discord bar mark or run: omarchy-shell quickshell.discord.panel toggle"
