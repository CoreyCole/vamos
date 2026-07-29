# Opaque settlement — Slice 1 handoff

## Done

- Branch: `opaque-settlement-launch-boundary`
- Commit: branch tip `opaque-settlement-launch-boundary` — `feat(hermes): define opaque settlement launch boundary`
- Retained managed thread validation and manager registration before Pi spawn.
- Made launch identity injection authoritative and de-duplicated for `VAMOS_PLAN_DIR`, `VAMOS_THOUGHTS_ROOT`, `PI_SESSION_ID`, and `VAMOS_HERMES_THREAD_ID`; an inherited thread ID is removed when no owner is provided.
- Updated only managed prompts: settlement is system-recorded; completion commands are prohibited; fenced YAML/YML is optional opaque evidence without required schema, outcome, or successor. Manual prompts retain their legacy completion instruction.

## Verification

- Passed: `go test ./cmd/vamos-runtime/internal/hermescmd`
- Passed: `git diff --check`
- Blocked baseline package: `go test ./cmd/vamos-runtime/internal/hermescmd ./cmd/vamos-runtime/internal/qrspicmd` fails because `cmd/vamos-runtime/internal/qrspicmd` references absent `LockKey`, `FileStateStore`, `ManagerState`, and `deps`; the touched `hermescmd` package passes.

## Scope boundary

Slice 1 only. No settlement persistence, fences, extension behavior, server work, completion transport, or Slice 2+ implementation was changed.

## Next

Proceed with the separately planned Slice 2 persistence/fence/extension/server work from its approved artifacts.
