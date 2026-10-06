#!/bin/sh
# Run portable, actual-consumer keyboard checks with inert models.
set -eu
exec env QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software /usr/lib/qt6/bin/qmltestrunner \
  -input "$(dirname "$0")/../../tests/visual"
