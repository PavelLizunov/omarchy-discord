#!/bin/sh
# Offscreen contract harness for Panel.qml's Esc ladder / guild entry and
# Service.qml's per-guild channel restore. Builds a scratch config root of
# symlinks (nothing is written into the plugin tree) and runs it headless.
#
#   components/harness/run-panel.sh      # exits non-zero on the first failure
#
# XDG_RUNTIME_DIR is redirected at a scratch directory so the Service mounted
# here can never reach the real backend socket; its DaemonManager is inert
# anyway (no plugin dir), so the socket client is never even wanted.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
shell=${OMARCHY_SHELL_DIR:-/usr/share/omarchy/shell}
root=${HARNESS_ROOT:-${TMPDIR:-/tmp}/omarchy-discord-panel-harness}
rm -rf "$root"
mkdir -p "$root/runtime"
ln -s "$shell/Commons" "$root/Commons"
ln -s "$shell/Ui" "$root/Ui"
for f in components Api.js Emoji.js Keymap.js Markdown.js Panel.qml Service.qml \
  BackendClient.qml DaemonManager.qml DiscordIcon.qml; do
  ln -s "$repo/$f" "$root/$f"
done
ln -s "$here/MockService.qml" "$root/MockService.qml"
# The real switcher is a layer-shell PanelWindow, which has no backend under
# offscreen; Service.qml's Component needs the type to resolve even though it
# never instantiates it.
ln -s "$here/QuickSwitchStub.qml" "$root/QuickSwitch.qml"
ln -s "$here/panel.qml" "$root/shell.qml"
XDG_RUNTIME_DIR="$root/runtime"
export XDG_RUNTIME_DIR
QT_QPA_PLATFORM=offscreen
export QT_QPA_PLATFORM
exec qs -p "$root" "$@"
