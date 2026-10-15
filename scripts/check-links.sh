#!/usr/bin/env bash
# scripts/check-links.sh — validasi tautan relatif antar dokumen.
# External http(s) links: hanya dilaporkan (tidak di-fetch agar offline-safe).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
fail=0
while IFS= read -r -d '' f; do
  # shellcheck disable=SC2016
  links=$(grep -oE '\[[^]]*\]\(([^)]+)\)' "$f" | sed -E 's/.*\(([^)]+)\)/\1/' || true)
  for link in $links; do
    case "$link" in
      http://*|https://*|\#*|mailto:*)
        echo "external/anchor: $f -> $link"
        ;;
      *)
        target="$link"
        target="${target%%\#*}"
        base=$(dirname "$f")
        if [ ! -e "$base/$target" ]; then
          echo "BROKEN: $f -> $link"
          fail=1
        fi
        ;;
    esac
  done
done < <(find "$ROOT" -name '*.md' -not -path '*/target/*' -not -path '*/node_modules/*' -print0)
if [ "$fail" -ne 0 ]; then
  echo "link check FAILED"
  exit 1
fi
echo "link check OK"
