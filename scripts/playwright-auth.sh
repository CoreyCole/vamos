#!/usr/bin/env bash
set -euo pipefail

TARGET_URL="${1:-http://localhost:4200/agent-chat}"
TOKEN="${CN_AGENTS_PLAYWRIGHT_AUTH_TOKEN:-}"

case "$TARGET_URL" in
  http://localhost:4200/*|http://127.0.0.1:4200/*) ;;
  *) echo "playwright-auth only accepts local cn-agents URLs, got: $TARGET_URL" >&2; exit 2 ;;
esac

encoded_redirect="$(python3 - <<'PY' "$TARGET_URL"
import sys, urllib.parse
u = urllib.parse.urlparse(sys.argv[1])
path = u.path or "/"
if u.query:
    path += "?" + u.query
print(urllib.parse.quote(path, safe=""))
PY
)"

auth_url="http://localhost:4200/internal/playwright-auth?redirect=${encoded_redirect}"
if [ -n "$TOKEN" ]; then
  auth_url="${auth_url}&token=$(python3 - <<'PY' "$TOKEN"
import sys, urllib.parse
print(urllib.parse.quote(sys.argv[1], safe=""))
PY
)"
fi

playwright-cli open --browser=chromium "$auth_url"
final_url="$(playwright-cli eval --browser=chromium 'window.location.href' | tail -n 1)"
case "$final_url" in
  */login*|*/internal/playwright-auth*)
    echo "Playwright auth failed; final URL: $final_url" >&2
    exit 1
    ;;
esac

echo "Playwright authenticated at: $final_url"
