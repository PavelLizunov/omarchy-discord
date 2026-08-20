#!/usr/bin/env bash
# Install the bundled prebuilt backend when one matches this machine, otherwise
# build it with Go. Exit 30 when neither is possible.
set -euo pipefail

source_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
runtime_dir=${OMARCHY_DISCORD_RUNTIME_DIR:-"$HOME/.local/lib/omarchy-discord"}
destination="$runtime_dir/omarchy-discord-backend"
architecture=$(uname -m)
prebuilt="$source_root/backend/dist/$architecture/omarchy-discord-backend"

# Build outside the plugin directory: Omarchy hot-reloads a plugin whenever any
# file inside it changes, so build output inside the tree would make the
# recursive watcher reload the plugin on every write and kill the build.
cache_root=${XDG_CACHE_HOME:-"$HOME/.cache"}
target_dir="$cache_root/omarchy-discord/target"
go_cache="${GOCACHE:-"$cache_root/omarchy-discord/gocache"}"

if [[ -x $prebuilt ]]; then
  install -d -m 700 -- "$runtime_dir"
  install -m 755 -- "$prebuilt" "$destination"
  printf 'Installed bundled Discord backend: %s\n' "$destination"
  exit 0
fi

command -v go >/dev/null 2>&1 || {
  echo "build-backend.sh: no bundled backend for $architecture and the Go toolchain is missing" >&2
  exit 30
}

[[ -f $source_root/backend/go.mod ]] || {
  echo "build-backend.sh: backend/go.mod not found; nothing to build" >&2
  exit 30
}

install -d -m 700 -- "$target_dir"
(
  cd -- "$source_root/backend"
  GOCACHE="$go_cache" GOFLAGS="${GOFLAGS:-} -trimpath" CGO_ENABLED=0 \
    go build -ldflags='-s -w' -o "$target_dir/omarchy-discord-backend" \
    ./cmd/omarchy-discord-backend
)
install -d -m 700 -- "$runtime_dir"
install -m 755 -- "$target_dir/omarchy-discord-backend" "$destination"
printf 'Built and installed Discord backend: %s\n' "$destination"
