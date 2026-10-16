#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
python3 -B "$root/tests/stage-dropped-files.py"
g++ -std=c++17 -fPIC "$root/tests/drop/drop-test.cpp" -o "$tmp/drop-test" \
  $(pkg-config --cflags --libs Qt6Quick Qt6Qml Qt6Test)
QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software \
  timeout 12s "$tmp/drop-test" "$root/tests/drop/Preview.qml"
printf 'PASS: native file-drop consumer\n'
