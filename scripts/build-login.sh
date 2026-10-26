#!/usr/bin/env bash
set -euo pipefail

source_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
runtime_dir=${OMARCHY_DISCORD_RUNTIME_DIR:-"$HOME/.local/lib/omarchy-discord"}
for tool in qmake6 make; do
  command -v "$tool" >/dev/null || {
    echo "build-login.sh: $tool is required; install base-devel, qt6-base and qt6-webengine" >&2
    exit 1
  }
done
build_dir=$(mktemp -d "${TMPDIR:-/tmp}/omarchy-discord-login.XXXXXX")
trap 'rm -rf -- "$build_dir"' EXIT
(cd "$build_dir" && qmake6 "$source_root/auth/login.pro" && make -j2)
install -d -m 700 -- "$runtime_dir"
install -m 755 -- "$build_dir/omarchy-discord-login" "$runtime_dir/omarchy-discord-login"
printf 'Installed Discord sign-in helper: %s\n' "$runtime_dir/omarchy-discord-login"
