#!/usr/bin/env bash
set -euo pipefail

failed=0
while IFS= read -r -d '' file; do
  if ! grep -Fq 'github.com/charmbracelet/bubbletea' "$file"; then
    echo "$file is a TUI implementation but does not use Bubble Tea" >&2
    failed=1
  fi
done < <(find internal/delivery/cli -type f -name '*_tui.go' ! -name '*_test.go' -print0)

if grep -RIn -E '\\x1b\[(H|2J)' internal/delivery/cli --include='*.go' --exclude='*_test.go'; then
  echo 'manual full-screen ANSI clearing is forbidden; use Bubble Tea' >&2
  failed=1
fi

exit "$failed"
