#!/usr/bin/env bash
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

staged_paths=$(git diff --cached --name-only --diff-filter=ACMR)
if [[ -z "$staged_paths" ]]; then
    exit 0
fi

has_path() {
    grep -Eq "$1" <<<"$staged_paths"
}

require_version() {
    local tool=$1 expected=$2 actual
    actual=$($tool version)
    if [[ "$actual" != "$expected" ]]; then
        echo "pre-commit: $tool $expected required; found $actual" >&2
        exit 1
    fi
}

sqlc_inputs='^(sqlc\.yaml|pkg/db/queries/|pkg/db/migrations/schema\.sql$)'
templ_inputs='\.templ$'
go_inputs='(^go\.(mod|sum)$|\.go$)'
run_sqlc=false
run_templ=false

if has_path "$sqlc_inputs"; then
    run_sqlc=true
    require_version sqlc v1.31.1
    sqlc generate
    git diff --exit-code -- pkg/db
fi

if has_path "$templ_inputs"; then
    run_templ=true
    require_version templ v0.3.1001
    templ generate
    git diff --exit-code -- ':(glob)**/*_templ.go'
fi

if $run_sqlc || $run_templ || has_path "$go_inputs"; then
    binary=$(mktemp "${TMPDIR:-/tmp}/vamos-pre-commit.XXXXXX")
    trap 'rm -f "$binary"' EXIT
    go build -o "$binary" ./cmd/server
fi
