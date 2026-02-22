#!/usr/bin/env bash
set -euo pipefail

payload="$(cat)"

if printf '%s' "$payload" | grep -Eiq 'git (reset --hard|clean -f|checkout \.|restore \.|branch -D)|rm -rf /|mkfs|:\(\)\{:\|:&\};:|dd if=.* of=/dev/'; then
  echo "Blocked destructive shell command by project policy." >&2
  exit 2
fi

exit 0
