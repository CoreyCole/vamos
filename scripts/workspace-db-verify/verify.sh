#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Bump this version when verifier CLI behavior changes enough to force rebuild everywhere.
VERIFY_VERSION="v1"
VERIFY_BIN="$SCRIPT_DIR/workspace-db-verify-${VERIFY_VERSION}"

needs_build=0
if [ ! -f "$VERIFY_BIN" ]; then
  needs_build=1
else
  if { find "$REPO_ROOT/cmd/vamos-runtime" "$REPO_ROOT/pkg/ctl/verifycmd" \
      -type f -name '*.go' -newer "$VERIFY_BIN" -print -quit; \
    find "$REPO_ROOT" -maxdepth 1 \( -name 'go.mod' -o -name 'go.sum' \) \
      -newer "$VERIFY_BIN" -print -quit; } | grep -q .; then
    needs_build=1
  fi
fi

if [ "$needs_build" -eq 1 ]; then
  echo "Building workspace DB verifier ${VERIFY_VERSION}..." >&2
  (cd "$REPO_ROOT" && go build -o "$VERIFY_BIN" ./cmd/vamos-runtime)
  find "$SCRIPT_DIR" -maxdepth 1 -name 'workspace-db-verify-*' -type f \
    ! -name "workspace-db-verify-${VERIFY_VERSION}" -delete
fi

exec "$VERIFY_BIN" ctl verify db-workspaces "$@"
