#!/usr/bin/env bash
# Fails when data that must be identical in the backend and the frontend
# drifts apart (TECH_SPEC §6.2 and §9): demo dossiers and Balas Sopan
# templates. Every copy must be byte-identical to its canonical file.
set -euo pipefail

status=0
check() {
  local canonical="$1"; shift
  for copy in "$@"; do
    if ! diff -q "$canonical" "$copy" >/dev/null; then
      echo "FAIL: $copy differs from $canonical"
      diff "$canonical" "$copy" | head -20
      status=1
    fi
  done
}

check backend/internal/demo/demo-cases.json frontend/src/lib/data/demo-cases.json

if [[ $status -eq 0 ]]; then
  echo "demo-cases.json in sync"
fi
exit $status
