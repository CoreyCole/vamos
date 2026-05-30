# DatastarUI Copied Source

This directory is maintained by the DatastarUI copy/update workflow.

Rules for agents:

- Do not make Vamos-specific customizations inside copied component internals.
- Reusable primitive/default fixes go to `context/datastarui` first.
- After upstream edits, run `go run ./context/datastarui/cmd/datastarui update --source ./context/datastarui --target ./pkg/datastarui --module github.com/CoreyCole/vamos` from the Vamos repo root.
- Put Vamos-specific theme/composition changes in `static/css/*`, `server/...`, or helpers outside `pkg/datastarui`.
- Before committing copied updates, run `go run ./context/datastarui/cmd/datastarui diff --source ./context/datastarui --target ./pkg/datastarui --module github.com/CoreyCole/vamos` and `go run ./context/datastarui/cmd/datastarui doctor --target ./pkg/datastarui --module github.com/CoreyCole/vamos`.
- Emergency local patches are temporary only: document the reason in the implementation handoff and upstream them into `context/datastarui` before final review when possible.
