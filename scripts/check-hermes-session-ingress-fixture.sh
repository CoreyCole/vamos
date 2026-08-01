#!/usr/bin/env bash
set -euo pipefail

readonly expected_commit="e77efc4b6d543f8948026405ee9f7863e7a11900"
readonly expected_sha256="d88f77ea8226cc6d55be144569a75d10baa54e47ab69c6131ed3dbac19a0256b"
readonly relative_fixture="tests/fixtures/session_ingress_protocol_v1.json"
readonly repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly mirror="$repo_root/pkg/hermes/testdata/session_ingress_protocol_v1.json"

if [[ $# -ne 1 ]]; then
	printf 'usage: %s HERMES_CHECKOUT\n' "$0" >&2
	exit 2
fi

readonly checkout="$1"
if [[ ! -d "$checkout" ]] || ! git -C "$checkout" rev-parse --git-dir >/dev/null 2>&1; then
	printf 'Hermes checkout is absent or is not a Git checkout: %s\n' "$checkout" >&2
	exit 1
fi

readonly actual_commit="$(git -C "$checkout" rev-parse HEAD)"
if [[ "$actual_commit" != "$expected_commit" ]]; then
	printf 'Hermes checkout is at %s; expected H0 %s\n' "$actual_commit" "$expected_commit" >&2
	exit 1
fi

readonly source_fixture="$checkout/$relative_fixture"
if [[ ! -f "$source_fixture" ]] || [[ ! -f "$mirror" ]]; then
	printf 'fixture is absent from Hermes checkout or Vamos mirror\n' >&2
	exit 1
fi

fixture_sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

readonly source_sha256="$(fixture_sha256 "$source_fixture")"
readonly mirror_sha256="$(fixture_sha256 "$mirror")"
if [[ "$source_sha256" != "$expected_sha256" ]] || [[ "$mirror_sha256" != "$expected_sha256" ]]; then
	printf 'fixture SHA-256 mismatch: Hermes=%s Vamos=%s expected=%s\n' \
		"$source_sha256" "$mirror_sha256" "$expected_sha256" >&2
	exit 1
fi

cmp "$source_fixture" "$mirror"
printf 'Hermes H0 fixture matches Vamos mirror (%s)\n' "$expected_sha256"
