# DatastarUI dependency development

Vamos consumes `github.com/coreycole/datastarui` as a normal Go module dependency pinned in `go.mod`.

Do not commit a local `replace github.com/coreycole/datastarui => ...` directive in Vamos. External consumers build from the pinned module version in `go.mod`.

## Temporary local workspace workflow

For local parallel development, create a temporary Go workspace outside committed files:

```bash
cd /path/to/parent
go work init ./vamos ./vamos/context/datastarui
```

Delete or ignore the generated `go.work` when you are done. It is a local override, not a repository contract.

## Temporary uncommitted replace workflow

For short-lived local testing, an uncommitted replace is also acceptable:

```bash
go mod edit -replace github.com/coreycole/datastarui=./context/datastarui
```

Before committing, remove it:

```bash
go mod edit -dropreplace github.com/coreycole/datastarui
```

Hosts that force `GOWORK=off` ignore ambient `go.work`; those hosts must either build against the pinned DatastarUI version or own a private build override outside Vamos.
