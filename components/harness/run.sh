#!/bin/sh
# Run the Timeline harness against the installed Omarchy shell's qs.Commons /
# qs.Ui. Builds a scratch config root (symlinks only) so nothing is written
# into the plugin tree, then `qs -p` it.
#
#   components/harness/run.sh            # run in the foreground
#   omarchy-shell is NOT involved; this is a plain quickshell instance.
# IPC while running (ROOT is printed on start):
#   qs -p "$ROOT" ipc call harness state|prepend|append|toggleActive|markRead|focus
#   qs -p "$ROOT" ipc call harness key j      # synthesized key (see pressKey)
# Stop: qs kill -p "$ROOT"
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
ln -s "$repo/Markdown.js" "$root/Markdown.js"
ln -s "$here/shell.qml" "$root/shell.qml"
echo "harness root: $root"
exec qs -p "$root" "$@"
