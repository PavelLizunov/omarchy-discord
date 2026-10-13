#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
runner=${QML_TEST_RUNNER:-/usr/lib/qt6/bin/qmltestrunner}
command -v xvfb-run >/dev/null || { echo 'xvfb-run is required for isolated QML interaction checks' >&2; exit 2; }
[[ -x $runner ]] || { echo 'Qt 6 qmltestrunner is required' >&2; exit 2; }
cd -- "$root"
# Match the native GTK activation semantics without connecting to the desktop.
QT_QPA_PLATFORM=xcb QT_QPA_PLATFORMTHEME=gtk3 QT_QUICK_BACKEND=software
export QT_QPA_PLATFORM QT_QPA_PLATFORMTHEME QT_QUICK_BACKEND
exec timeout 90s xvfb-run -a -s '-screen 0 1280x1024x24 -nolisten tcp -noreset -extension GLX' \
  "$runner" -input tests/visual -import .
