#!/usr/bin/env bash
set -euo pipefail

: "${PORT:?PORT must be set by Vamos or by the direct-run command}"
: "${VAMOS_APP_FILES_ROOT:=files}"

mkdir -p "$VAMOS_APP_FILES_ROOT"

exec uv run --frozen streamlit run app.py \
  --server.address=127.0.0.1 \
  --server.port="$PORT" \
  --server.headless=true \
  --server.enableCORS=false \
  --server.enableXsrfProtection=false \
  --browser.gatherUsageStats=false
