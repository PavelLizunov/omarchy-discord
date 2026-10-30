#!/usr/bin/env bash
# Source before Go builds/tests; the runtime binary finds libdave beside itself.
native_root=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/$(uname -m)" && pwd)
export PKG_CONFIG_PATH="$native_root/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
export LD_LIBRARY_PATH="$native_root/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -Wl,-rpath,\$ORIGIN"
unset native_root
