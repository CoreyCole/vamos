# Opaque settlement — Slice 2 handoff

## Done

- Branch: `opaque-settlement-persistence`
- Commit identity is recorded by the manager-owned plan handoff after this self-contained Graphite slice is created.
- Added v1 `OpaqueSettlementEnvelope` JSON structural decoding, exact settlement paths, and immutable exact-byte `.json` publication/read APIs.
- Pending recovery returns original file bytes as base64; Go does not serialize settlement JSON.
- Added shared checked-in JS/Go fixtures covering zero, one, and many opaque fences, including multiline, empty, non-ASCII, and CRLF values.
- Preserved legacy `PiResult` and checkpoint behavior. Artifact discovery continues to distinguish legacy results, legacy checkpoints, and opaque settlement paths.

## Verification

- `go test ./cmd/vamos-runtime/internal/hermescmd`
- `node --test .pi/extensions/q-manager-child/opaque-settlement-fixtures.test.js`
- `git diff --check`
- `gt branch info` reports parent `opaque-settlement-launch-boundary`.
- The implementation and handoff were squashed into this one Graphite slice; working tree was clean after the commit.

## Scope boundary

Slice 2 persistence only. No callbacks, registration, completion transport, extension lifecycle behavior, server delivery, YAML parsing, or Slice 3+ lexical capture was added.

## Next

Do not begin further slices until separately authorized.
