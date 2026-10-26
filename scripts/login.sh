#!/usr/bin/env bash
set -euo pipefail
runtime_dir=${OMARCHY_DISCORD_RUNTIME_DIR:-"$HOME/.local/lib/omarchy-discord"}
helper="$runtime_dir/omarchy-discord-login"
if [[ ! -x $helper ]]; then
  echo 'Discord sign-in helper is missing. Run scripts/build-login.sh from the plugin checkout.' >&2
  exit 2
fi
# The sign-in browser must retain Chromium's sandbox and default TLS validation.
unset QTWEBENGINE_DISABLE_SANDBOX QTWEBENGINE_CHROMIUM_FLAGS QTWEBENGINE_REMOTE_DEBUGGING
exec "$helper"
