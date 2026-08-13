#!/usr/bin/env bash
set -euo pipefail

max_lines="${GODEX_MAX_GO_FILE_LINES:-400}"
failed=0

while IFS= read -r -d '' file; do
  lines="$(wc -l < "$file" | tr -d ' ')"
  if (( lines > max_lines )); then
    echo "$file has $lines lines; limit is $max_lines" >&2
    failed=1
  fi
done < <(find cmd internal -type f -name '*.go' ! -name '*_test.go' -print0)

exit "$failed"
