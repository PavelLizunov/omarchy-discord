#!/bin/sh
set -eu
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
shell=${OMARCHY_SHELL_DIR:-/usr/share/omarchy/shell}
root=${HARNESS_ROOT:-${XDG_RUNTIME_DIR:-/tmp}/omarchy-discord-harness}
rm -rf "$root"
mkdir -p "$root"
ln -s "$shell/Commons" "$root/Commons"
ln -s "$shell/Ui" "$root/Ui"
ln -s "$repo/components" "$root/components"
ln -s "$repo/Api.js" "$root/Api.js"
ln -s "$repo/Emoji.js" "$root/Emoji.js"
ln -s "$repo/Markdown.js" "$root/Markdown.js"
ln -s "$here/shell.qml" "$root/shell.qml"
echo "harness root: $root"
exec qs -p "$root" "$@"
