# DatastarUI Copied Source

This directory is maintained by the DatastarUI copy/update workflow.

Rules for agents:

- Do not make Vamos-specific customizations inside copied component internals.
- Reusable primitive/default fixes go to sibling checkout `../datastarui` first.
- After upstream edits, run `VAMOS_ROOT=$PWD; (cd ../datastarui && go run ./cmd/datastarui update --source . --target "$VAMOS_ROOT/pkg/datastarui" --module github.com/CoreyCole/vamos)` from the Vamos repo root.
- Put Vamos-specific theme/composition changes in `static/css/*`, `server/...`, or helpers outside `pkg/datastarui`.
- Before committing copied updates, run `VAMOS_ROOT=$PWD; (cd ../datastarui && go run ./cmd/datastarui diff --source . --target "$VAMOS_ROOT/pkg/datastarui" --module github.com/CoreyCole/vamos)` and `VAMOS_ROOT=$PWD; (cd ../datastarui && go run ./cmd/datastarui doctor --target "$VAMOS_ROOT/pkg/datastarui" --module github.com/CoreyCole/vamos)`.
- Emergency local patches are temporary only: document the reason in the implementation handoff and upstream them into `../datastarui` before final review when possible.
