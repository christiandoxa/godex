#!/usr/bin/env bash
set -euo pipefail

max_lines="${GODEX_MAX_GO_FILE_LINES:-400}"
baseline_file="${GODEX_SOURCE_SIZE_BASELINE:-scripts/source-size-baseline.txt}"
failed=0

declare -A baseline=()
if [[ -f "$baseline_file" ]]; then
  while IFS=$'\t' read -r limit file; do
    [[ -z "$limit" || "$limit" == \#* ]] && continue
    if [[ ! "$limit" =~ ^[0-9]+$ || -z "$file" ]]; then
      echo "invalid source-size baseline entry: $limit$file" >&2
      exit 2
    fi
    baseline["$file"]="$limit"
  done < "$baseline_file"
fi

while IFS= read -r -d '' file; do
  lines="$(wc -l < "$file" | tr -d ' ')"
  limit="$max_lines"
  baseline_limit="${baseline[$file]:-}"
  if [[ -n "$baseline_limit" ]]; then
    limit="$baseline_limit"
    if (( limit < max_lines )); then
      limit="$max_lines"
    fi
    if (( lines < baseline_limit && lines > max_lines )); then
      echo "$file shrank to $lines lines; lower its baseline from $baseline_limit to $lines" >&2
      failed=1
      continue
    fi
    if (( lines <= max_lines )); then
      echo "$file is now within the $max_lines-line default; remove its baseline entry" >&2
      failed=1
      continue
    fi
  fi
  if (( lines > limit )); then
    echo "$file has $lines lines; limit is $limit" >&2
    failed=1
  fi
done < <(find cmd internal -type f -name '*.go' ! -name '*_test.go' -print0)

for file in "${!baseline[@]}"; do
  if [[ ! -f "$file" ]]; then
    echo "source-size baseline references missing file $file" >&2
    failed=1
  fi
done

exit "$failed"
