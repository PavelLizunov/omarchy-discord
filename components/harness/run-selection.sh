#!/bin/sh
# Offscreen contract harness for selectable message text, the Ctrl+C copy of a
# selection and the hovered-link copy chip. Builds a scratch config root of
# symlinks (nothing is written into the plugin tree) and runs it headless.
#
#   components/harness/run-selection.sh   # exits non-zero on the first failure
#
# Every pointer event is synthesized by QtTest inside the harness window; the
# live Wayland session is never touched.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
shell=${OMARCHY_SHELL_DIR:-/usr/share/omarchy/shell}
root=${HARNESS_ROOT:-${TMPDIR:-/tmp}/omarchy-discord-selection-harness}
rm -rf "$root"
mkdir -p "$root"
ln -s "$shell/Commons" "$root/Commons"
ln -s "$shell/Ui" "$root/Ui"
ln -s "$repo/components" "$root/components"
ln -s "$repo/Api.js" "$root/Api.js"
ln -s "$repo/Emoji.js" "$root/Emoji.js"
ln -s "$repo/Markdown.js" "$root/Markdown.js"
ln -s "$here/selection.qml" "$root/shell.qml"
QT_QPA_PLATFORM=offscreen
export QT_QPA_PLATFORM
exec qs -p "$root" "$@"
