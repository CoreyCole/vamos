# DatastarUI development workflow

Vamos dogfoods DatastarUI as a shadcn-like copy consumer. Copied DatastarUI defaults live in `pkg/datastarui` and are tracked with `pkg/datastarui/datastarui.lock.json`.

## Agent rules

- Do not hand-customize `pkg/datastarui/components/**` for Vamos-only behavior.
- Reusable primitive/default fixes go to sibling checkout `../datastarui` first.
- After upstream edits, run the copy/update CLI into `pkg/datastarui`.
- Vamos-specific theme/composition belongs in `static/css/*`, `server/...`, or app helpers outside `pkg/datastarui`.
- Before committing copied source, run `datastarui diff` and `datastarui doctor`.

## Update copied components

From the Vamos repo root, run the CLI in the sibling DatastarUI checkout and point it back at the Vamos copied-source target. Capturing `VAMOS_ROOT` keeps `templ generate` scoped to Vamos so it does not recurse into the upstream checkout.

```bash
VAMOS_ROOT=$PWD
(cd ../datastarui && go run ./cmd/datastarui update \
  --source . \
  --target "$VAMOS_ROOT/pkg/datastarui" \
  --module github.com/CoreyCole/vamos)

templ generate

(cd ../datastarui && go run ./cmd/datastarui diff \
  --source . \
  --target "$VAMOS_ROOT/pkg/datastarui" \
  --module github.com/CoreyCole/vamos)

(cd ../datastarui && go run ./cmd/datastarui doctor \
  --target "$VAMOS_ROOT/pkg/datastarui" \
  --module github.com/CoreyCole/vamos)
```

Expected clean state:

- `diff` prints the lock path and no drift entries.
- `doctor` exits successfully.
- `templ generate` refreshes copied component generated Go files after copied `.templ` updates.

## Emergency patch policy

Temporary local patches inside `pkg/datastarui` are allowed only to unblock verification. Document the reason in the implementation handoff, upstream the fix into `../datastarui`, rerun `update`, and make `diff` clean before final review whenever possible.

## Local upstream development

Use sibling checkout `../datastarui` for active DatastarUI edits. Do not commit a `replace github.com/coreycole/datastarui => ...` directive as the Vamos consumer contract.

`pkg/datastarui` should stay CLI-managed copied source. App theme tokens and page composition stay in Vamos-owned files outside that directory.
