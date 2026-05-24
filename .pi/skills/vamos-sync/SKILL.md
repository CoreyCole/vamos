---
name: vamos-sync
description: Sync and restack a Vamos Graphite stack onto latest main. Use when asked to run gt sync, restack onto new main commits, fix Graphite restack conflicts, continue a conflicted restack, or update a Vamos feature/workspace branch after main moved.
---

# Vamos Sync

Sync a Vamos Graphite stack onto latest `main`, resolve conflicts, and leave the stack ready for tests/review. This skill updates/restacks only; it does not merge, submit, push, or restart services.

## Invariants

- Use in a Vamos runtime checkout or copied Vamos implementation workspace, never in `../vamos-main`.
- Preserve stack shape. Do not squash, cherry-pick, patch-apply, or rename branches.
- Do not use `git rebase --continue`; use `gt continue` for Graphite restacks.
- Do not run `/vamos-merge`, `gt submit`, or `git push` unless the user explicitly asks after sync succeeds.
- If tracked files are dirty before syncing, stop and ask whether to commit/stash/revert. Do not hide user changes.

## Step 1: Preflight

```bash
pwd
git rev-parse --show-toplevel
git branch --show-current
git status --short
gt log short
```

Rules:

- Stop if this is `../vamos-main` or any clean baseline checkout.
- Stop for dirty tracked files unless the user explicitly wants those changes included before sync.
- Record the current branch; restack this branch, not an guessed stack top.

```bash
source_branch=$(git branch --show-current)
test -n "$source_branch"
```

## Step 2: Sync latest main and restack

```bash
git fetch origin +refs/heads/main:refs/remotes/origin/main
gt sync --no-interactive
gt restack --branch "$source_branch" --no-interactive
```

If this completes without conflicts, continue to Step 5.

## Step 3: Conflict loop

When Graphite stops for conflicts:

```bash
git status --short
git diff --name-only --diff-filter=U
git status
```

Resolve conflicts with these rules:

- Prefer preserving both main's new behavior and the stack's intended behavior.
- Read conflicted files before editing; do not blindly accept ours/theirs.
- Generated files:
  - `*_templ.go`: resolve `.templ` first, then run `templ generate`.
  - sqlc outputs: resolve schema/query files first, then run the repo's sqlc/build path (`just build --no-restart` is acceptable when unsure).
  - generated E2E files under `pkg/e2e/generated`: regenerate from stories; do not hand-edit unless explicitly fixing generated runtime.
- If a conflict changes behavior beyond a mechanical merge, note the decision in the final summary.

After resolving all conflict markers:

```bash
rg -n '<<<<<<<|=======|>>>>>>>' .
git status --short
git add <resolved-files>
gt continue --no-interactive
```

If `gt continue` stops again, repeat Step 3. Never skip/drop commits without explicit user approval.

## Step 4: Verify conflict resolutions

Run the narrowest tests covering conflicted areas. Defaults for Vamos runtime stacks:

```bash
go test ./server/config ./server/services/workspaces ./server/services/agentchat ./cmd/build-agents/internal/build
```

If generated assets changed or conflict scope is broad:

```bash
templ generate
go test ./pkg/agents/workflows/runtime ./pkg/release ./server/config ./server/services/workspaces ./pkg/db ./cmd/build-agents/internal/build
just build --no-restart
```

Fix failures before declaring sync complete. If a fix changes tracked files, commit/amend it onto the branch Graphite is currently restacking with `gt modify --no-interactive` only after the restack is no longer in progress.

## Step 5: Final checks

```bash
git status --short
gt log short
git merge-base --is-ancestor origin/main HEAD
git log --format='%h %s' --reverse origin/main..HEAD
git diff --stat origin/main..HEAD
```

Expected:

- No uncommitted tracked files unless the user explicitly requested a no-commit sync.
- Current branch stack sits above `origin/main`.
- Commit list and diff stat still match the intended Vamos work.

## Response

Report concisely:

- branch synced/restacked
- conflict files and resolution decisions, if any
- verification commands/results
- remaining uncommitted files or follow-up risks
- next action: review/test/merge, not push/submit unless requested
