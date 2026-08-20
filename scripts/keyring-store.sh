#!/bin/sh
set -eu
IFS= read -r user_token
[ -z "$user_token" ] && exit 3
printf '%s' "$user_token" | secret-tool store \
  --label='Omarchy Discord user token' \
  service quickshell-discord \
  kind user-token
