---
date: 2026-05-23T16:04:21-07:00
researcher: creative-mode-agent
last_updated_by: creative-mode-agent
git_commit: 7588fa42278322ec51b8ba0515bd0f66a441517e
branch: main
repository: cn-agents
stage: plan
ticket: Port durable chat session architecture and E2E Story Playwright-Go work into Vamos
plan_dir: thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
---

# Implementation Plan: Port Durable Chat + E2E Stack to Vamos

## Status

- [x] Slice 1: Port inventory + conflict ledger
- [x] Slice 2: Durable chat/session/schema reconciliation
- [x] Slice 3: Vamos runtime CLI + ctl compatibility
- [ ] Slice 4: Story parser, selector catalog, step catalog, check command
- [ ] Slice 5: Deterministic generator + checked-in generated tests
- [ ] Slice 6: Playwright runtime, auth, run artifacts
- [ ] Slice 7: Workspace fixtures + durable-chat scenario helpers
- [ ] Slice 8: Viewport/property matrices + QRSPI plan bundles
- [ ] Slice 9: Semantic goldens, visual review, bounded repair
- [ ] Slice 10: Docs, verification contract, implementation handoff

## Implementation Workspace Prep

`/q-workspace` will create or repair the fresh filesystem copy for `/q-implement` after `/q-review thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md` succeeds.

Planned workspace path:

```text
/home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
```

Workspace base selection was completed by `/q-workspace` on 2026-05-23 16:16 PDT. This is a normal parent plan with no prior implementation stack and no requested continuation of an unmerged Graphite stack. Selected base is the clean Vamos baseline checkout `/home/ruby/cn/chestnut-flake/vamos-main` on `main` at `24fcb6fa48088fe541e72668929627efef0c44cf`. The active implementation workspace is `/home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos`; it is a copied Vamos repository, not a cn-agents workspace. For review-fixes plans under `reviews/*_implementation-review/`, `/q-workspace` must prove whether the parent implementation stack top is merged into `origin/main`; if not, it must base the workspace on that parent top branch/commit and record the expected `gt parent` for review-fix slice branches.

Do not use `git worktree`. This workspace is a normal copied directory created with efficient filesystem clone/reflink copy (`cp -a --reflink=auto` on Linux; `cp -ac` on macOS). If the workspace directory is dirty or missing when implementation starts, stop and ask before moving/replacing it. Move aside only with an explicit backup name after confirmation.

Repository submission model: this is Vamos runtime work. Implement in the workspace selected by `/q-workspace`, then create a Graphite branch for each tracked edit slice at the end of that slice (`gt create port-durable-chat-e2e-to-vamos_slice-N` or reviewed equivalent) after implementation and verification. Commit slices with Conventional Commit messages plus QRSPI XML footers. Run `/vamos-merge` after implementation, implementation review, and verification are complete. Do not commit implementation slices directly to `main` and do not pre-create future slice branches.

The full plan directory must exist inside the workspace at the same relative path:

```text
thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
```

Implementation starts with:

```bash
/q-workspace thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md
```

## Common Source Paths and Port Rules

Use these constants in every slice:

```bash
OLD_REPO=/home/ruby/cn/chestnut-flake/cn-agents-2026-05-19_16-21-15_vamos-durable-session-chat-architecture
OLD_ROOT=$OLD_REPO/pkg/agents
NEW_ROOT=$(pwd) # inside /q-workspace-created Vamos copy
PLAN_DIR=thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
```

Path rewrites:

```text
$OLD_ROOT/cmd/vamos-runtime     -> cmd/vamos-runtime
$OLD_ROOT/cmd/vamos-launcher    -> cmd/vamos-launcher
$OLD_ROOT/pkg/ctl               -> pkg/ctl
$OLD_ROOT/pkg/e2e               -> pkg/e2e
$OLD_ROOT/docs/features         -> docs/features
$OLD_ROOT/server/services/agentchat -> server/services/agentchat (semantic merge only)
$OLD_ROOT/pkg/db                -> pkg/db (schema/query semantic merge + regenerate)
```

Porting rules:

- Prefer old stack behavior by ADR-001, but adapt to current Vamos root layout, `VAMOS_*`, `.vamos`, YAML config, and host-owned thoughts root.
- Keep `.agents` as the shared symlink only. Do not copy private/shared `.agents/skills/*` into Vamos. If a Vamos-specific reusable Pi skill/prompt is required, place it under `../vamos/.pi/skills/` or `.pi/prompts/`.
- Generated E2E tests are derived from `docs/features/*.story.md`. Regenerate; do not hand-edit generated files.
- Use Go style helpers during semantic edits: `pkg/pointers.To`, `pkg/collections.Set`, nullable `Ptr()`, `pkg/checked`, and generated `pkg/db` enum types.
- Preserve old unstaged review-fix deltas, but fix `chat_steps.go` advisory lint: `testing.TB` parameter names and the magic `500` polling interval.

______________________________________________________________________

## Slice 1: Port inventory + conflict ledger

### Files

- `thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/context/implement/port-inventory.md` (new)
- `thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/AGENTS.md` (modify if new durable gotcha is discovered)

### Changes

Create a factual inventory before source edits. Use commands, not guesses:

```bash
mkdir -p "$PLAN_DIR/context/implement"
{
  echo "# Port Inventory: Durable Chat + E2E to Vamos"
  echo
  echo "## Source"
  git -C "$OLD_ROOT" rev-parse --show-toplevel
  git -C "$OLD_ROOT" branch --show-current
  git -C "$OLD_ROOT" rev-parse HEAD
  git -C "$OLD_REPO" status --short
  echo
  echo "## Target"
  pwd
  git branch --show-current
  git rev-parse HEAD
  git status --short
  echo
  echo "## Old E2E/CLI files"
  find "$OLD_ROOT/cmd/vamos-runtime" "$OLD_ROOT/cmd/vamos-launcher" "$OLD_ROOT/pkg/ctl" "$OLD_ROOT/pkg/e2e" "$OLD_ROOT/docs/features" -type f | sort | sed "s#$OLD_ROOT/##"
  echo
  echo "## Target pre-existing matching paths"
  find cmd pkg docs -path 'cmd/vamos-runtime' -o -path 'cmd/vamos-launcher' -o -path 'pkg/ctl' -o -path 'pkg/e2e' -o -path 'docs/features' 2>/dev/null
  echo
  echo "## Durable chat semantic-merge candidates"
  for rel in \
    server/services/agentchat/chat_session_handlers.go \
    server/services/agentchat/chat_session_integration.go \
    server/services/agentchat/document_workspace.go \
    server/services/agentchat/embedded_chat.go \
    server/services/agentchat/session_import.go \
    server/services/agentchat/workflows/state_store.go \
    server/services/agentchat/workspace_models.go \
    pkg/db/migrations/schema.sql \
    pkg/db/queries/impl_workspaces.sql; do
      echo "### $rel"
      test -f "$OLD_ROOT/$rel" && git -C "$OLD_ROOT" log --oneline -- "$rel" | head -5 || true
      test -f "$NEW_ROOT/$rel" && git -C "$NEW_ROOT" log --oneline -- "$rel" | head -5 || true
  done
} > "$PLAN_DIR/context/implement/port-inventory.md"
```

Then append a hand-written `## Conflict Ledger` section with this minimum table:

```markdown
| Area | Old path | New path | Decision | Rationale |
|---|---|---|---|---|
| E2E CLI | cmd/vamos-runtime | cmd/vamos-runtime | copy_new | absent in target |
| E2E packages | pkg/e2e | pkg/e2e | copy_new_then_lint | absent in target; generated tests regenerated later |
| ctl | pkg/ctl | pkg/ctl | copy_new | absent in target; `cmd/agentsctl` can reuse later if needed |
| durable schema | pkg/db/*impl_workspaces* | pkg/db/* | semantic_merge | old deletes `impl_workspaces`; target still has it; ADR says old stack authoritative |
| embedded freeform | server/services/agentchat/embedded_chat.go | same | semantic_merge | old unstaged fix required by refresh/resume story |
```

If the inventory finds target files that make old code obsolete, record them here before changing source.

### Tests

No code tests. Validate inventory is non-empty and names the old unstaged deltas:

```bash
grep -E 'durable-session-chat.story.md|chat_steps.go|embedded_chat.go|impl_workspaces' "$PLAN_DIR/context/implement/port-inventory.md"
git diff -- "$PLAN_DIR/context/implement/port-inventory.md"
```

### Verify

```bash
test -s "$PLAN_DIR/context/implement/port-inventory.md"
grep -q 'Conflict Ledger' "$PLAN_DIR/context/implement/port-inventory.md"
git status --short
```

### Commit Message

```text
docs(qrspi): inventory old durable chat e2e port slice 1

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="1">Port inventory + conflict ledger</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 2: Durable chat/session/schema reconciliation

### Files

- `pkg/db/migrations/schema.sql` (modify)
- `pkg/db/queries/*.sql` (modify/delete obsolete impl workspace query files after compare)
- `pkg/db/*.sql.go` (regenerate)
- `pkg/agents/chatsession/*` (semantic merge only if target is missing old behavior)
- `server/services/agentchat/chat_session_handlers.go` (semantic merge only)
- `server/services/agentchat/chat_session_integration.go` (semantic merge only)
- `server/services/agentchat/document_workspace.go` (semantic merge old unstaged deltas)
- `server/services/agentchat/embedded_chat.go` (semantic merge old unstaged freeform fix)
- `server/services/agentchat/session_import.go` (semantic merge old unstaged deltas)
- `server/services/agentchat/workflows/state_store.go` (semantic merge old unstaged deltas)
- `server/services/agentchat/workspace_models.go` (semantic merge old unstaged deltas)

### Changes

1. Compare existing target durable chat with old stack before copying:

```bash
for rel in \
  pkg/db/migrations/schema.sql \
  server/services/agentchat/chat_session_handlers.go \
  server/services/agentchat/chat_session_integration.go \
  server/services/agentchat/document_workspace.go \
  server/services/agentchat/embedded_chat.go \
  server/services/agentchat/session_import.go \
  server/services/agentchat/workflows/state_store.go \
  server/services/agentchat/workspace_models.go; do
  mkdir -p "$PLAN_DIR/context/implement/diffs/$(dirname "$rel")"
  diff -u "$NEW_ROOT/$rel" "$OLD_ROOT/$rel" > "$PLAN_DIR/context/implement/diffs/$rel.diff" || true
done
```

2. Reconcile schema in `pkg/db/migrations/schema.sql`:

- Keep current `plan_workspaces` if target code still uses it.
- Remove obsolete `impl_workspaces` table and related indexes only if no current target code still reads it after the old query/model port. Run:

```bash
rg 'impl_workspaces|ImplWorkspace|impl workspace|impl_workspace' pkg server cmd
```

- If references remain only in generated sqlc files/old queries, delete old query file(s) and regenerate.
- If current target has intentional new `plan_workspaces` behavior, keep it and update any old E2E fixture/workspace code to use `workspaces`/`plan_workspaces`, not resurrect `impl_workspaces`.

3. Apply old embedded freeform fix to `server/services/agentchat/embedded_chat.go`. Ensure the persisted freeform selection path renders the freeform right rail. The final logic should look like this shape, adapted to current target names:

```go
if selectedWorkspace != nil && selectedWorkspace.IsFreeform() {
    return EmbeddedFreeformRightRailContent(EmbeddedFreeformRightRailData{
        ChatSession: session,
        Selection:   selectedWorkspace,
        Messages:    messages,
    })
}
```

Use actual target types; do not introduce `IsFreeform()` if the target already represents freeform with an enum/nullable field. Prefer generated DB enum constants over raw strings.

4. Apply old workspace/latest-chat restoration deltas in the five unstaged agentchat files. Preserve shared workspace semantics: no creator-only filters. Any user email is provenance, not workspace access control.

1. Regenerate database code using the repo's existing command. Try in order and keep the command that works in the handoff:

```bash
just sqlc
# or, if justfile has no sqlc recipe:
go generate ./pkg/db/...
# or:
sqlc generate
```

6. Run Go formatting only on touched Go files:

```bash
gofmt -w \
  server/services/agentchat/chat_session_handlers.go \
  server/services/agentchat/chat_session_integration.go \
  server/services/agentchat/document_workspace.go \
  server/services/agentchat/embedded_chat.go \
  server/services/agentchat/session_import.go \
  server/services/agentchat/workflows/state_store.go \
  server/services/agentchat/workspace_models.go \
  pkg/db/*.go
```

### Tests

Add or preserve focused tests for:

- Workspace current chat session creation/restoration.
- Snapshot/SSE/command handler behavior still compiles and passes.
- Freeform embedded refresh/resume fix if a unit-level renderer test exists; otherwise slice 7 E2E covers it.

Suggested current tests to run and update as needed:

```bash
go test ./pkg/db/... ./pkg/agents/chatsession/... ./server/services/agentchat
```

If `impl_workspaces` removal changes generated query names, update compile errors in packages that import those generated functions. Do not leave dead generated files.

### Verify

```bash
rg 'impl_workspaces|ImplWorkspace' pkg/db server pkg cmd || true
go test ./pkg/db/... ./pkg/agents/chatsession/... ./server/services/agentchat
git diff --check
```

### Commit Message

```text
feat(agentchat): reconcile durable chat schema and sessions slice 2

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="2">Durable chat/session/schema reconciliation</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 3: Vamos runtime CLI + ctl compatibility

### Files

- `cmd/vamos-launcher/main.go` (new)
- `cmd/vamos-launcher/bootstrap.go` (new)
- `cmd/vamos-runtime/main.go` (new)
- `cmd/vamos-runtime/internal/rootcmd/root.go` (new)
- `cmd/vamos-runtime/internal/rootcmd/root_test.go` (new)
- `cmd/vamos-runtime/internal/ctlcmd/root.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/root.go` (new minimal command group; subcommand implementations may arrive in later slices)
- `cmd/vamos-runtime/internal/e2ecmd/root_test.go` (new)
- `pkg/ctl/verifycmd/*` (new)
- `pkg/ctl/workspacecmd/*` (new)
- `cmd/agentsctl/main.go` (modify only if target should delegate to `pkg/ctl`)
- `go.mod`, `go.sum` (modify for `github.com/spf13/cobra` if not already present)

### Changes

Copy old CLI/ctl roots, then adapt to current Vamos root:

```bash
mkdir -p cmd/vamos-runtime cmd/vamos-launcher pkg/ctl
cp -a "$OLD_ROOT/cmd/vamos-runtime/." cmd/vamos-runtime/
cp -a "$OLD_ROOT/cmd/vamos-launcher/." cmd/vamos-launcher/
cp -a "$OLD_ROOT/pkg/ctl/." pkg/ctl/
```

Keep this root command body:

```go
package rootcmd

import (
    "github.com/spf13/cobra"

    "github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/ctlcmd"
    "github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/e2ecmd"
)

func NewCommand() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "vamos",
        Short: "Vamos Agents developer CLI",
        Long:  "Managed CLI for Vamos Agents workspace operations and story E2E workflows.",
    }
    cmd.AddCommand(ctlcmd.NewCommand())
    cmd.AddCommand(e2ecmd.NewCommand())
    return cmd
}
```

Keep `cmd/vamos-runtime/main.go` simple:

```go
package main

import (
    "fmt"
    "os"

    "github.com/CoreyCole/vamos/cmd/vamos-runtime/internal/rootcmd"
)

func main() {
    if err := rootcmd.NewCommand().Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Adapt `pkg/ctl/workspacecmd` from legacy names:

- `.cn-agents` -> `.vamos`
- `CN_AGENTS_*` -> `VAMOS_*`
- process/service names must come from config or CLI flags when possible; no hardcoded Chestnut host defaults.
- If `cmd/agentsctl` remains, make it a compatibility entrypoint that imports `pkg/ctl/...` and points users to `vamos ctl ...` without duplicating logic.

Temporarily keep `e2e` subcommands wired but allow `not implemented` only for subcommands whose implementation is added in later slices. By slice 9, no `notImplemented` command should remain.

Run:

```bash
go mod tidy
gofmt -w cmd/vamos-runtime cmd/vamos-launcher pkg/ctl cmd/agentsctl
```

### Tests

Port and adapt old tests:

- `cmd/vamos-runtime/internal/rootcmd/root_test.go`
- `cmd/vamos-runtime/internal/e2ecmd/root_test.go`
- `pkg/ctl/verifycmd/workspaces_test.go`
- `pkg/ctl/workspacecmd/workspacecmd_test.go`

Assertions must use `.vamos` and `VAMOS_*` names.

### Verify

```bash
go test ./cmd/vamos-runtime/... ./cmd/vamos-launcher/... ./pkg/ctl/...
go run ./cmd/vamos-runtime --help
go run ./cmd/vamos-runtime ctl workspace --help
go run ./cmd/vamos-runtime e2e --help
! rg 'CN_AGENTS|\.cn-agents|REPO_PATH|MARKDOWN_BASE_PATH' cmd/vamos-runtime cmd/vamos-launcher pkg/ctl cmd/agentsctl
```

### Commit Message

```text
feat(cli): add vamos runtime and ctl shell slice 3

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="3">Vamos runtime CLI + ctl compatibility</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 4: Story parser, selector catalog, step catalog, check command

### Files

- `docs/features/durable-session-chat.story.md` (new)
- `docs/features/thoughts-workbench.story.md` (new if still present in old stack)
- `pkg/e2e/story/types.go` (new)
- `pkg/e2e/story/parse.go` (new; include old unstaged parser tweaks)
- `pkg/e2e/story/validate.go` (new)
- `pkg/e2e/story/*_test.go` (new)
- `pkg/e2e/selectors/catalog.go` (new)
- `pkg/e2e/selectors/catalog_test.go` (new)
- `pkg/e2e/steps/catalog.go` (new; include old unstaged catalog tweaks)
- `pkg/e2e/steps/catalog_test.go` (new)
- `pkg/e2e/steps/noop_steps.go` (new)
- `pkg/e2e/fixtures/registry.go` (new minimal registry if needed by validation)
- `cmd/vamos-runtime/internal/e2ecmd/check.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/check_test.go` (new)

### Changes

Copy parser/catalog/check foundations:

```bash
mkdir -p docs/features pkg/e2e/story pkg/e2e/selectors pkg/e2e/steps pkg/e2e/fixtures cmd/vamos-runtime/internal/e2ecmd
cp "$OLD_ROOT/docs/features/"*.story.md docs/features/
cp -a "$OLD_ROOT/pkg/e2e/story/." pkg/e2e/story/
cp -a "$OLD_ROOT/pkg/e2e/selectors/." pkg/e2e/selectors/
cp "$OLD_ROOT/pkg/e2e/steps/catalog.go" pkg/e2e/steps/catalog.go
cp "$OLD_ROOT/pkg/e2e/steps/catalog_test.go" pkg/e2e/steps/catalog_test.go
cp "$OLD_ROOT/pkg/e2e/steps/noop_steps.go" pkg/e2e/steps/noop_steps.go
cp "$OLD_ROOT/pkg/e2e/fixtures/registry.go" pkg/e2e/fixtures/registry.go
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/check.go" cmd/vamos-runtime/internal/e2ecmd/check.go
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/check_test.go" cmd/vamos-runtime/internal/e2ecmd/check_test.go
```

Preserve this story model shape:

```go
package story

type Feature struct {
    Slug          string
    Title         string
    UserStory     string
    BusinessRules []string
    Scenarios     []Scenario
    Properties    []Property
    SourcePath    string
}

type Scenario struct {
    Slug     string
    Title    string
    Viewport string
    Given    []Step
    When     []Step
    Then     []Step
}

type Step struct {
    Kind   StepKind
    Verb   StepVerb
    Args   map[string]string
    Source SourceRange
}

type StepResolver interface{ ResolveStep(step Step) error }
type FixtureResolver interface{ HasFixture(name string) bool }
```

Preserve the old parse improvements from unstaged `pkg/e2e/story/parse.go`: scenario/source-line handling must accept the new durable-chat story text and not lose source ranges.

Adapt package paths only; imports should remain `github.com/CoreyCole/vamos/...`.

Make `RunCheck` parse stories, validate steps against `steps.DefaultCatalog()`, validate fixtures against `fixtures.DefaultRegistry()`, and check generated freshness only if generated files exist. The body should follow this shape:

```go
func RunCheck(ctx context.Context, cfg CheckConfig) error {
    features, err := story.ParseDir(cfg.StoryDir, story.ParseOptions{Strict: true})
    if err != nil {
        return err
    }
    catalog := steps.DefaultCatalog()
    registry := fixtures.DefaultRegistry()
    for _, feature := range features {
        if err := story.ValidateFeature(feature, catalog, registry); err != nil {
            return err
        }
    }
    fmt.Fprintf(os.Stdout, "validated %d feature(s)\n", len(features))
    return nil
}
```

Use actual old code if stronger; keep no arbitrary Playwright selector generation here.

### Tests

Add/port tests that cover:

- Parses `durable-session-chat.story.md` and `thoughts-workbench.story.md`.
- Unsupported step fails validation with source line.
- New freeform/workspace latest-chat step verbs are in the catalog.
- `vamos e2e check` succeeds on `docs/features`.

### Verify

```bash
gofmt -w pkg/e2e/story pkg/e2e/selectors pkg/e2e/steps pkg/e2e/fixtures cmd/vamos-runtime/internal/e2ecmd
go test ./pkg/e2e/story ./pkg/e2e/selectors ./pkg/e2e/steps ./pkg/e2e/fixtures ./cmd/vamos-runtime/internal/e2ecmd
go run ./cmd/vamos-runtime e2e check
```

### Commit Message

```text
feat(e2e): add story validation and check command slice 4

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="4">Story parser, selector catalog, step catalog, check command</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 5: Deterministic generator + checked-in generated tests

### Files

- `pkg/e2e/generate/check.go` (new)
- `pkg/e2e/generate/generate.go` (new)
- `pkg/e2e/generate/write.go` (new)
- `pkg/e2e/generate/generate_test.go` (new)
- `pkg/e2e/generated/durable_session_chat_e2e_test.go` (new generated)
- `pkg/e2e/generated/thoughts_workbench_e2e_test.go` (new generated if story exists)
- `cmd/vamos-runtime/internal/e2ecmd/generate.go` (new)
- `.gitignore` (modify for runtime artifacts only; do not ignore generated tests)

### Changes

Copy generator code, then regenerate tests from stories:

```bash
mkdir -p pkg/e2e/generate pkg/e2e/generated
cp -a "$OLD_ROOT/pkg/e2e/generate/." pkg/e2e/generate/
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/generate.go" cmd/vamos-runtime/internal/e2ecmd/generate.go
```

Preserve this generation core:

```go
func Generate(features []story.Feature, opts Options) (Result, error) {
    if opts.PackageName == "" {
        opts.PackageName = "generated"
    }
    if opts.StepCatalog == nil {
        opts.StepCatalog = steps.DefaultCatalog()
    }
    sort.Slice(features, func(i, j int) bool { return features[i].Slug < features[j].Slug })

    result := Result{}
    for _, feature := range features {
        content, err := renderFeature(feature, opts)
        if err != nil {
            return Result{}, err
        }
        filename := strings.ReplaceAll(feature.Slug, "-", "_") + "_e2e_test.go"
        result.Files = append(result.Files, GeneratedFile{
            Path:    filepath.Join(opts.OutputDir, filename),
            Content: content,
        })
    }
    return result, nil
}
```

Generated tests must contain only:

- `e2e.RunScenario(...)` / `e2e.RunScenarioWithViewport(...)`
- `steps.<CuratedHelper>(t, ctx, ...)`

They must not contain raw Playwright selectors, `page.Locator`, or ad-hoc browser code.

Update `.gitignore` only for local artifacts such as:

```gitignore
.e2e-runs/
pkg/e2e/artifacts/tmp/
```

Do not ignore `pkg/e2e/generated/*.go`.

Regenerate:

```bash
go run ./cmd/vamos-runtime e2e generate
```

### Tests

Port generator tests and add assertions:

```go
func TestGenerateUsesCuratedStepsOnly(t *testing.T) {
    // Parse docs/features, generate to memory, assert output contains RunScenario and steps.,
    // and does not contain playwright.Page or Locator.
}
```

### Verify

```bash
gofmt -w pkg/e2e/generate pkg/e2e/generated cmd/vamos-runtime/internal/e2ecmd
go test ./pkg/e2e/generate ./pkg/e2e/generated ./cmd/vamos-runtime/internal/e2ecmd
go run ./cmd/vamos-runtime e2e generate --check
! rg 'Locator\(|playwright\.Page|page\.' pkg/e2e/generated
git diff --check
```

### Commit Message

```text
feat(e2e): generate story-derived go tests slice 5

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="5">Deterministic generator + checked-in generated tests</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 6: Playwright runtime, auth, run artifacts

### Files

- `pkg/e2e/runtime/config.go` (new)
- `pkg/e2e/runtime/scenario.go` (new)
- `pkg/e2e/runtime/auth.go` (new)
- `pkg/e2e/runtime/artifacts.go` (new)
- `pkg/e2e/runtime/timeout.go` (new)
- `pkg/e2e/runtime/*_test.go` (new)
- `pkg/e2e/artifacts/manifest.go` (new)
- `pkg/e2e/artifacts/report.go` (new)
- `pkg/e2e/artifacts/screenshots.go` (new)
- `pkg/e2e/artifacts/*_test.go` (new)
- `pkg/e2e/steps/browser_steps.go` or old equivalent if present (new)
- `cmd/vamos-runtime/internal/e2ecmd/run.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/run_test.go` (new)
- `go.mod`, `go.sum` (modify for Playwright-Go deps)

### Changes

Copy runtime/artifact/run code:

```bash
mkdir -p pkg/e2e/runtime pkg/e2e/artifacts
cp -a "$OLD_ROOT/pkg/e2e/runtime/." pkg/e2e/runtime/
cp -a "$OLD_ROOT/pkg/e2e/artifacts/." pkg/e2e/artifacts/
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/run.go" cmd/vamos-runtime/internal/e2ecmd/run.go
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/run_test.go" cmd/vamos-runtime/internal/e2ecmd/run_test.go
```

Preserve runtime API:

```go
type RuntimeConfig struct {
    RepoRoot     string
    BaseURL      string
    AuthToken    string
    Workspace    WorkspaceIdentity
    ArtifactsDir string
    Headless     bool
    Viewports    []ViewportClass
}

type WorkspaceIdentity struct {
    Slug         string
    CheckoutPath string
    DBPath       string
    ManagerURL   string
}

func LoadConfigFromEnv(cwd string) (RuntimeConfig, error)
func RunScenario(t *testing.T, featureSlug, scenarioSlug string, fn ScenarioFunc)
func RunScenarioWithViewport(t *testing.T, featureSlug, scenarioSlug string, viewport ViewportClass, fn ScenarioFunc)
```

Adapt env/config:

- Use `VAMOS_BASE_URL`, `VAMOS_E2E_AUTH_TOKEN`, `VAMOS_WORKSPACE_SLUG`, `VAMOS_WORKSPACE_CHECKOUT`, `VAMOS_DATABASE_PATH`, `VAMOS_WORKSPACE_MANAGER_URL`.
- Read `.vamos/run/workspace.env`; never `.cn-agents/run/workspace.env`.
- Refuse canonical main DB unless an explicit safe test flag exists in old code; keep workspace-only safety.
- Do not mint Chestnut-specific auth. Auth helper should consume a token/header/cookie from config or no-op for local dev modes already supported by Vamos.

`RunE2E` should build a `go test` command for `./pkg/e2e/generated` with `-run` filters from story/scenario/viewport, write artifacts under `--artifacts-dir` or `.e2e-runs/<timestamp>`, and preserve run manifest paths.

Run:

```bash
go mod tidy
gofmt -w pkg/e2e/runtime pkg/e2e/artifacts cmd/vamos-runtime/internal/e2ecmd
```

### Tests

Port runtime/artifact tests. Add/keep tests for:

- `LoadConfigFromEnv` reads `.vamos/run/workspace.env` and `VAMOS_*`.
- `RunE2E` refuses unsafe main/canonical DB when fixtures are enabled.
- Run manifest writes stable JSON with story/scenario/status/artifact paths.

### Verify

```bash
go test ./pkg/e2e/runtime ./pkg/e2e/artifacts ./pkg/e2e/steps ./cmd/vamos-runtime/internal/e2ecmd
go test ./pkg/e2e/generated -run TestDoesNotExist || true
go run ./cmd/vamos-runtime e2e run --help
! rg 'CN_AGENTS|\.cn-agents|REPO_PATH|MARKDOWN_BASE_PATH' pkg/e2e/runtime pkg/e2e/artifacts cmd/vamos-runtime/internal/e2ecmd
```

### Commit Message

```text
feat(e2e): add playwright runtime and run artifacts slice 6

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="6">Playwright runtime, auth, run artifacts</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 7: Workspace fixtures + durable-chat scenario helpers

### Files

- `pkg/e2e/fixtures/*.go` (new/modify)
- `pkg/e2e/fixtures/*_test.go` (new/modify)
- `pkg/e2e/runtime/workspace.go` (modify)
- `pkg/e2e/runtime/workspace_test.go` (modify)
- `pkg/e2e/steps/fixture_steps.go` (new if in old source)
- `pkg/e2e/steps/chat_steps.go` (new; include old unstaged review-fix helpers)
- `pkg/e2e/steps/fixture_steps_test.go` (new)
- `docs/features/durable-session-chat.story.md` (modify; include old unstaged scenarios)
- `pkg/e2e/generated/durable_session_chat_e2e_test.go` (regenerate)

### Changes

Copy fixture/chat helper files, then apply old unstaged review fixes and lint cleanup:

```bash
cp -a "$OLD_ROOT/pkg/e2e/fixtures/." pkg/e2e/fixtures/
cp "$OLD_ROOT/pkg/e2e/runtime/workspace.go" pkg/e2e/runtime/workspace.go
cp "$OLD_ROOT/pkg/e2e/runtime/workspace_test.go" pkg/e2e/runtime/workspace_test.go
cp "$OLD_ROOT/pkg/e2e/steps/chat_steps.go" pkg/e2e/steps/chat_steps.go
cp "$OLD_ROOT/pkg/e2e/steps/fixture_steps_test.go" pkg/e2e/steps/fixture_steps_test.go 2>/dev/null || true
cp "$OLD_ROOT/docs/features/durable-session-chat.story.md" docs/features/durable-session-chat.story.md
```

Required helper surface:

```go
func DefaultRegistry() Registry
func Load(ctx context.Context, db DBTX, workspace WorkspaceIdentity, name string) (State, error)
func ReadWorkspaceEnv(checkout string) (WorkspaceIdentity, error)
func OpenWorkspaceDB(ctx context.Context, cfg RuntimeConfig) (*sql.DB, error)
func OpenThoughtsRootChat(t testing.TB, ctx *runtime.Context)
func SendFreeformChatPrompt(t testing.TB, ctx *runtime.Context, prompt string)
func SeedLatestWorkspaceChats(t testing.TB, ctx *runtime.Context, fixture string)
func OpenSeededWorkspaceChat(t testing.TB, ctx *runtime.Context, workspace string)
```

Fix old lint while porting:

```go
const chatPollInterval = 500 * time.Millisecond

func OpenThoughtsRootChat(t testing.TB, ctx *runtime.Context) {
    t.Helper()
    // old body, but parameter is named t, not tb/thelper
}
```

Adapt helper code:

- `.cn-agents` -> `.vamos`.
- Use `VAMOS_*` env fields from `server/services/workspaces/process.go`.
- Use target DB generated types/enums, not raw strings, when inserting chat/session rows.
- Use `nullable.T.Ptr()` and `pointers.To` instead of local pointer helpers.
- Fixture DB writes must target the workspace DB only; refuse canonical main DB.

Regenerate generated tests:

```bash
go run ./cmd/vamos-runtime e2e generate
```

Generated durable tests must include:

- `TestDurableSessionChat_FreeformChatStartedFromThoughtsRootSurvivesRefreshAndResume`
- `TestDurableSessionChat_WorkspaceSwitchingRestoresEachWorkspaceLatestChat`

### Tests

Add/port tests for:

- Fixture registry recognizes chat fixtures.
- Workspace env reads `.vamos/run/workspace.env`.
- Chat step catalog resolves freeform and latest-workspace verbs.
- Generated durable tests compile.

### Verify

```bash
gofmt -w pkg/e2e/fixtures pkg/e2e/runtime pkg/e2e/steps pkg/e2e/generated
go test ./pkg/e2e/fixtures ./pkg/e2e/runtime ./pkg/e2e/steps ./pkg/e2e/generated
go run ./cmd/vamos-runtime e2e check
go run ./cmd/vamos-runtime e2e generate --check
rg 'FreeformChatStartedFromThoughtsRootSurvivesRefreshAndResume|WorkspaceSwitchingRestoresEachWorkspaceLatestChat' pkg/e2e/generated/durable_session_chat_e2e_test.go
! rg 'time\.Millisecond \* 500|500 \* time\.Millisecond|thelper' pkg/e2e/steps/chat_steps.go
```

### Commit Message

```text
feat(e2e): add workspace fixtures and durable chat steps slice 7

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="7">Workspace fixtures + durable-chat scenario helpers</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 8: Viewport/property matrices + QRSPI plan bundles

### Files

- `pkg/e2e/runtime/viewports.go` (new/modify)
- `pkg/e2e/runtime/viewports_test.go` (new/modify)
- `pkg/e2e/story/properties.go` (new/modify)
- `pkg/e2e/story/properties_test.go` (new/modify)
- `pkg/e2e/generate/properties.go` (new/modify if old source has separate file; otherwise generator changes in `generate.go`)
- `pkg/e2e/artifacts/screenshots.go` (modify)
- `pkg/e2e/artifacts/plan_bundle.go` (new)
- `pkg/e2e/artifacts/plan_bundle_test.go` (new)
- `docs/features/*.story.md` (modify property sections if old stack includes them)
- `cmd/vamos-runtime/internal/e2ecmd/run.go` (modify to export plan bundle)

### Changes

Copy old property/viewport/bundle files:

```bash
cp "$OLD_ROOT/pkg/e2e/runtime/viewports.go" pkg/e2e/runtime/viewports.go
cp "$OLD_ROOT/pkg/e2e/runtime/viewports_test.go" pkg/e2e/runtime/viewports_test.go
cp "$OLD_ROOT/pkg/e2e/story/properties.go" pkg/e2e/story/properties.go
cp "$OLD_ROOT/pkg/e2e/story/properties_test.go" pkg/e2e/story/properties_test.go
cp "$OLD_ROOT/pkg/e2e/artifacts/plan_bundle.go" pkg/e2e/artifacts/plan_bundle.go
cp "$OLD_ROOT/pkg/e2e/artifacts/plan_bundle_test.go" pkg/e2e/artifacts/plan_bundle_test.go
cp "$OLD_ROOT/pkg/e2e/artifacts/screenshots.go" pkg/e2e/artifacts/screenshots.go
```

Preserve API:

```go
func DefaultViewports() map[ViewportClass]Viewport
func ResolveViewports(names []string) ([]Viewport, error)
func ExpandProperties(feature story.Feature) ([]story.Scenario, error)
func ExportPlanBundle(ctx context.Context, manifest RunManifest, opts PlanBundleOptions) (PlanBundle, error)
```

Plan bundle behavior:

- If `--plan-dir thoughts/...` is passed to `vamos e2e run`, write an index under:

```text
thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/context/implement/e2e-runs/<run-id>/index.md
```

- Include run manifest, report, failures JSON, screenshots, HTML snapshots, trace paths, and exact command.
- Do not copy huge trace binaries into `thoughts`; link artifact paths.

Property expansion must remain deterministic: sort dimensions/values by story order, no random fuzzing.

### Tests

Port old viewport/property/bundle tests. Add assertion that plan bundle rejects non-`thoughts/...` plan dirs or normalizes them safely under configured root.

### Verify

```bash
gofmt -w pkg/e2e/runtime pkg/e2e/story pkg/e2e/generate pkg/e2e/artifacts cmd/vamos-runtime/internal/e2ecmd
go test ./pkg/e2e/...
go run ./cmd/vamos-runtime e2e check
go run ./cmd/vamos-runtime e2e generate --check
go run ./cmd/vamos-runtime e2e run --help | rg -- '--plan-dir|--viewport|--artifacts-dir'
```

### Commit Message

```text
feat(e2e): add viewport matrices and plan bundles slice 8

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="8">Viewport/property matrices + QRSPI plan bundles</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 9: Semantic goldens, visual review, bounded repair

### Files

- `pkg/e2e/goldens/*.go` (new)
- `pkg/e2e/goldens/*_test.go` (new)
- `pkg/e2e/review/*.go` (new)
- `pkg/e2e/review/*_test.go` (new)
- `pkg/e2e/repair/*.go` (new)
- `pkg/e2e/repair/*_test.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/review.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/goldens.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/fix.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/review_goldens_test.go` (new)
- `cmd/vamos-runtime/internal/e2ecmd/fix_test.go` (new)
- Optional `.pi/skills/e2e-image-review/SKILL.md` only if a Vamos-specific reusable prompt is necessary; do not copy shared `.agents/skills`.

### Changes

Copy semantic review/goldens/repair code:

```bash
mkdir -p pkg/e2e/goldens pkg/e2e/review pkg/e2e/repair
cp -a "$OLD_ROOT/pkg/e2e/goldens/." pkg/e2e/goldens/
cp -a "$OLD_ROOT/pkg/e2e/review/." pkg/e2e/review/
cp -a "$OLD_ROOT/pkg/e2e/repair/." pkg/e2e/repair/
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/review.go" cmd/vamos-runtime/internal/e2ecmd/review.go
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/goldens.go" cmd/vamos-runtime/internal/e2ecmd/goldens.go
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/fix.go" cmd/vamos-runtime/internal/e2ecmd/fix.go
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/review_goldens_test.go" cmd/vamos-runtime/internal/e2ecmd/review_goldens_test.go
cp "$OLD_ROOT/cmd/vamos-runtime/internal/e2ecmd/fix_test.go" cmd/vamos-runtime/internal/e2ecmd/fix_test.go
```

Preserve surfaces:

```go
func Capture(ctx context.Context, run RunManifest, opts CaptureOptions) error
func Accept(ctx context.Context, run RunManifest, opts AcceptOptions) error
func RunVisualReview(ctx context.Context, input VisualReviewInput) (VisualReviewResult, error)
func WriteReviewMarkdown(path string, input VisualReviewInput, result VisualReviewResult) error
func BuildPlan(ctx context.Context, req RepairRequest) (RepairPlan, error)
func ValidatePlan(plan RepairPlan) error
```

Required semantics:

- `vamos e2e review --run <run>` writes `e2e-visual.md` with frontmatter including tool, run ID, baseline, verdict.
- If deterministic CLI lacks Pi visual adapter, verdict may be `needs-human-review`; this is expected and must be documented, not treated as failure.
- `goldens accept` must require `--human-approved`.
- `fix` must reject production code/story edits unless human approval is explicit. Default allowed scopes: selectors, steps, runtime, generated tests.
- Remove any remaining `notImplemented` subcommand from `cmd/vamos-runtime/internal/e2ecmd/root.go` unless genuinely kept as unused helper with no command path.

If a Vamos-specific Pi skill/prompt is needed, create minimal project-local content under `.pi/skills/e2e-image-review/SKILL.md` that points to `pkg/e2e/review` and semantic golden rules. Do not copy `/home/ruby/cn/chestnut-flake/.agents/skills/e2e-image-review` into Vamos.

### Tests

Port old tests. Add/keep tests for:

- Human approval required for golden accept.
- Repair validator rejects `server/**`, `docs/features/**`, and arbitrary source edits without explicit approval.
- Review markdown emits `needs-human-review` when visual adapter result is absent.

### Verify

```bash
gofmt -w pkg/e2e/goldens pkg/e2e/review pkg/e2e/repair cmd/vamos-runtime/internal/e2ecmd
go test ./pkg/e2e/goldens ./pkg/e2e/review ./pkg/e2e/repair ./cmd/vamos-runtime/internal/e2ecmd
go run ./cmd/vamos-runtime e2e review --help
go run ./cmd/vamos-runtime e2e goldens accept --help | rg human-approved
go run ./cmd/vamos-runtime e2e fix --help
! rg 'not implemented yet' cmd/vamos-runtime/internal/e2ecmd
! find . -path './.agents/skills/*' -type f | grep .
```

### Commit Message

```text
feat(e2e): add semantic goldens review and repair slice 9

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="9">Semantic goldens, visual review, bounded repair</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

______________________________________________________________________

## Slice 10: Docs, verification contract, implementation handoff

### Files

- `docs/e2e-story-testing.md` (new/port)
- `docs/workspaces-verification.md` (modify/port)
- `AGENTS.md` (modify with concise E2E workflow guidance if not already present)
- `thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/context/implement/e2e-runs/<run-id>/index.md` (new from run, if browser run executed)
- `thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/handoffs/<timestamp>_implementation-complete.md` (new)

### Changes

Copy/port docs:

```bash
cp "$OLD_ROOT/docs/e2e-story-testing.md" docs/e2e-story-testing.md 2>/dev/null || true
cp "$OLD_ROOT/docs/workspaces-verification.md" docs/workspaces-verification.md 2>/dev/null || true
```

If old docs are absent or stale, create `docs/e2e-story-testing.md` with these sections:

```markdown
# E2E Story Testing

## Model
- `.story.md` files in `docs/features` are canonical.
- `pkg/e2e/generated` is generated by `vamos e2e generate` and checked in.
- Generated tests call `pkg/e2e/runtime` and `pkg/e2e/steps` only.

## Commands
- `go run ./cmd/vamos-runtime e2e check`
- `go run ./cmd/vamos-runtime e2e generate --check`
- `go run ./cmd/vamos-runtime e2e run --story durable-session-chat --plan-dir thoughts/...`
- `go run ./cmd/vamos-runtime e2e review --run <run-dir> --plan-dir thoughts/...`

## Safety
- Browser runs with fixtures must use a registered non-main workspace.
- Runtime metadata uses `.vamos/run/workspace.env` and `VAMOS_*`.
- `e2e review` may produce `needs-human-review` until a Pi visual adapter is available.
- `e2e fix` is bounded to selectors, steps, runtime, and generated tests unless a human approves wider changes.
```

Update root `AGENTS.md` only with short durable rules already in the current target style:

```markdown
## Story E2E guidance

Use `vamos e2e check` for story/parser/generator validation. Use `vamos e2e run` only in a registered non-main workspace when fixtures are enabled; it refuses canonical main DBs. Generated files under `pkg/e2e/generated` are derived from `docs/features/*.story.md` and should be regenerated, not hand-edited. Visual review belongs to `vamos e2e review` and `e2e-image-review`, not deterministic test runs. `vamos e2e fix` is bounded to selectors, steps, runtime, and generated tests unless a human explicitly approves story/product changes.
```

Run final automated gate:

```bash
just build --no-restart
go test ./server/config ./server/services/workspaces ./server/services/agentchat ./cmd/build-agents/internal/build
go test ./pkg/e2e/... ./cmd/vamos-runtime/... ./cmd/vamos-launcher/... ./pkg/ctl/...
go run ./cmd/vamos-runtime e2e check
go run ./cmd/vamos-runtime e2e generate --check
```

If a registered non-main workspace exists and `VAMOS_*` env is configured, run focused durable chat smoke:

```bash
go run ./cmd/vamos-runtime e2e run \
  --story durable-session-chat \
  --scenario freeform-chat-started-from-thoughts-root-survives-refresh-and-resume \
  --plan-dir "$PLAN_DIR"

go run ./cmd/vamos-runtime e2e run \
  --story durable-session-chat \
  --scenario workspace-switching-restores-each-workspace-latest-chat \
  --plan-dir "$PLAN_DIR"

go run ./cmd/vamos-runtime e2e review --run <run-dir> --plan-dir "$PLAN_DIR"
```

If no safe workspace exists, do not force a canonical main DB run. Record `not run: no registered non-main workspace` in the handoff and leave browser run for `/q-verify`/lead engineer.

Create implementation-complete handoff with:

- Branch stack names and hashes.
- Commands run and pass/fail output snippets.
- Whether focused browser runs were executed.
- `e2e review` verdict (`pass` or `needs-human-review`).
- Manual notes: old stale duplicate Temporal/TS-worker recovery was manual; deterministic visual review may need human screenshot/HTML inspection.

### Tests

Docs are covered by final gate above. If docs include command examples, ensure command names match actual CLI help.

### Verify

```bash
just build --no-restart
go test ./server/config ./server/services/workspaces ./server/services/agentchat ./cmd/build-agents/internal/build
go test ./pkg/e2e/... ./cmd/vamos-runtime/... ./cmd/vamos-launcher/... ./pkg/ctl/...
go run ./cmd/vamos-runtime e2e check
go run ./cmd/vamos-runtime e2e generate --check
git diff --check
```

### Commit Message

```text
docs(e2e): document story workflow and final verification slice 10

<qrspi-commit>
  <workspace>vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos</workspace>
  <slice number="10">Docs, verification contract, implementation handoff</slice>
  <artifacts>
    <design>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/design.md</design>
    <outline>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/outline.md</outline>
    <plan>thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/plan.md</plan>
  </artifacts>
</qrspi-commit>
```

## Final Implementation Handoff Requirements

After all slices are complete and before `/q-review` in implementation mode, create:

```text
thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/handoffs/<timestamp>_implementation-complete.md
```

Then emit QRSPI completion XML from `/q-implement` with next:

```text
/q-review thoughts/creative-mode-agent/plans/2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos/handoffs/<timestamp>_implementation-complete.md
```
