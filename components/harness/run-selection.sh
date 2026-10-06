#!/bin/sh
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
