# DatastarUI development workflow

Vamos dogfoods DatastarUI as a shadcn-like copy consumer. Copied DatastarUI defaults live in `pkg/datastarui` and are tracked with `pkg/datastarui/datastarui.lock.json`.

## Agent rules

- Do not hand-customize `pkg/datastarui/components/**` for Vamos-only behavior.
- Reusable primitive/default fixes go to `context/datastarui` first.
- After upstream edits, run the copy/update CLI into `pkg/datastarui`.
- Vamos-specific theme/composition belongs in `static/css/*`, `server/...`, or app helpers outside `pkg/datastarui`.
- Before committing copied source, run `datastarui diff` and `datastarui doctor`.

## Update copied components

From the Vamos repo root:

```bash
go run ./context/datastarui/cmd/datastarui update \
  --source ./context/datastarui \
  --target ./pkg/datastarui \
  --module github.com/CoreyCole/vamos

templ generate

go run ./context/datastarui/cmd/datastarui diff \
  --source ./context/datastarui \
  --target ./pkg/datastarui \
  --module github.com/CoreyCole/vamos

go run ./context/datastarui/cmd/datastarui doctor \
  --target ./pkg/datastarui \
  --module github.com/CoreyCole/vamos
```

Expected clean state:

- `diff` prints the lock path and no drift entries.
- `doctor` exits successfully.
- `templ generate` refreshes copied component generated Go files after copied `.templ` updates.

## Emergency patch policy

Temporary local patches inside `pkg/datastarui` are allowed only to unblock verification. Document the reason in the implementation handoff, upstream the fix into `context/datastarui`, rerun `update`, and make `diff` clean before final review whenever possible.

## Local upstream development

Use `context/datastarui` for active DatastarUI edits. Do not commit a `replace github.com/coreycole/datastarui => ...` directive as the Vamos consumer contract.

`pkg/datastarui` should stay CLI-managed copied source. App theme tokens and page composition stay in Vamos-owned files outside that directory.
