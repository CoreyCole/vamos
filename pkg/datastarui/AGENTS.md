# DatastarUI Copied Source

This directory is maintained by the DatastarUI copy/update workflow.

Rules for agents:

- Do not make Vamos-specific customizations inside copied component internals.
- If a component/default bug is reusable, fix it first in `context/datastarui`.
- After upstream edits, run `cd context/datastarui && go run ./cmd/datastarui update --source . --target ../../pkg/datastarui --module github.com/CoreyCole/vamos`.
- Put Vamos-specific theme/composition changes in `static/css/*`, `server/...`, or helpers outside `pkg/datastarui`.
- Before committing copied updates, run `cd context/datastarui && go run ./cmd/datastarui diff --source . --target ../../pkg/datastarui --module github.com/CoreyCole/vamos` and `cd context/datastarui && go run ./cmd/datastarui doctor --target ../../pkg/datastarui --module github.com/CoreyCole/vamos`.
- Emergency local patches are temporary only: document the reason in the implementation handoff and upstream them into `context/datastarui` before final review when possible.
