---
name: vamos-merge
description: Land Vamos and paired DatastarUI work, sync origin and local baselines, then rebuild, restart, and verify both dogfood lanes. Use for /vamos-merge, requests to merge into vamos-main or origin, or deploying stage and main.
---

# Vamos Merge

Keep DatastarUI and the local stage/main pairs current, built, and running:

| Role | Source/config checkout | Runtime/copied checkout |
| --- | --- | --- |
| UI source | `../datastarui` | `../vamos/pkg/datastarui` |
| Stage | thoughts repo | `../vamos` |
| Main | main thoughts baseline | `../vamos-main` |

Run from the canonical `../vamos` checkout. Resolve host-owned paths, URLs, and restart commands from deployment config or operator input.

**A runtime merge includes deployment.** Requests to merge into `../vamos-main` or push `origin/main` also require the main rebuild and restart. Stop after source sync only when the user explicitly requests source-only work or defers deployment.

```bash
: "${VAMOS_THOUGHTS_REPO_CHECKOUT:?set the working thoughts repo checkout}"
: "${VAMOS_MAIN_THOUGHTS_REPO_CHECKOUT:?set its clean main baseline checkout}"
: "${VAMOS_STAGE_URL:?set the stage health-check URL}"
: "${VAMOS_MAIN_URL:?set the main health-check URL}"
: "${VAMOS_STAGE_RESTART_CMD:?set the host-owned stage restart command}"
: "${VAMOS_MAIN_RESTART_CMD:?set the host-owned main restart command}"
thoughts_repo=$VAMOS_THOUGHTS_REPO_CHECKOUT
main_thoughts_repo=$VAMOS_MAIN_THOUGHTS_REPO_CHECKOUT
```

Keep these variables for the workflow. In the current dogfood setup, `cn-agents` is the thoughts repo and its `-main` sibling is the clean baseline; other hosts supply equivalent checkouts. Invoking `/vamos-merge` authorizes commits for task-owned changes, fast-forward merges, pushes, builds, and restarts.

## Rules

- Commit only task-owned changes. Stop on unrelated or ambiguous changes.
- If the thoughts repo has only `thoughts/` changes, run `just sync-thoughts`. This is valid merge cleanup because it formats, commits, rebases, and pushes those artifacts. Make sure that the checkout is clean before continuing.
- All local checkouts must finish on `main` and up to date with `origin/main`.
- Treat `../vamos-main` and `$main_thoughts_repo` as clean baselines. Do not edit them directly.
- Preserve feature commits in Vamos and DatastarUI. Fast-forward only; do not squash, cherry-pick, or create merge commits.
- Update `pkg/datastarui` through the DatastarUI CLI; do not hand-copy or hand-customize copied components.
- Stop on conflicts, non-fast-forward updates, build failures, restart failures, or failed HTTP smoke checks.
- `just build --no-restart` compiles only. A push or fast-forward does not update a running server.
- Do not report a runtime merge complete until both lanes serve the rebuilt runtime after restart.
- If permissions block a restart, report deployment blocked. Do not treat source sync as deployment success.
- Do not run workspace DB checks, workspace refreshes, Temporal schedules, or broad log scans.

## 1. Land DatastarUI and refresh the copied source

If `../datastarui` has task-owned changes, commit them first. For a DatastarUI feature branch, run `gt sync` and `gt restack`, then fast-forward `main` to the final feature HEAD. Otherwise, update `main` directly.

```bash
cd ../datastarui
ui_branch=$(git branch --show-current)
git status --short
if test "$ui_branch" != main; then
  git fetch origin main
  gt sync --no-interactive
  gt restack --branch "$ui_branch" --no-interactive
  ui_head=$(git rev-parse HEAD)
  git switch main
  git pull --rebase origin main
  git merge --ff-only "$ui_head"
else
  git pull --rebase origin main
fi
git push origin main
ui_head=$(git rev-parse HEAD)
```

Refresh Vamos when the lock points at another DatastarUI commit or the copy has drift. Inspect any pending `pkg/datastarui` edits before allowing the CLI to replace them.

```bash
cd ../vamos
VAMOS_ROOT=$PWD
ui_head=$(git -C ../datastarui rev-parse HEAD)
locked_ui=$(jq -r .commit pkg/datastarui/datastarui.lock.json)
if test "$locked_ui" != "$ui_head" || ! (cd ../datastarui && go run ./cmd/datastarui diff \
  --source . --target "$VAMOS_ROOT/pkg/datastarui" --module github.com/CoreyCole/vamos); then
  (cd ../datastarui && go run ./cmd/datastarui update \
    --source . --target "$VAMOS_ROOT/pkg/datastarui" --module github.com/CoreyCole/vamos)
  templ generate
  (cd ../datastarui && go run ./cmd/datastarui diff \
    --source . --target "$VAMOS_ROOT/pkg/datastarui" --module github.com/CoreyCole/vamos)
  (cd ../datastarui && go run ./cmd/datastarui doctor \
    --target "$VAMOS_ROOT/pkg/datastarui" --module github.com/CoreyCole/vamos)
fi
```

Commit generated copied-source changes with the Vamos work before continuing.

## 2. Land and push the working checkouts

Inspect `../vamos` and the thoughts repo. Commit their task-owned changes before pulling. If Vamos is on a feature branch, run `/vamos-sync` before the commands below; the commands record its final HEAD and fast-forward `main` to it.

```bash
cd ../vamos
source_branch=$(git branch --show-current)
source_head=$(git rev-parse HEAD)
git status --short

if test "$source_branch" != main; then
  git switch main
fi

git pull --rebase origin main
if test "$source_branch" != main; then
  git merge --ff-only "$source_head"
fi
git push origin main

cd "$thoughts_repo"
git switch main
git status --short
git pull --rebase origin main
git push origin main
```

Both working checkouts must now be clean.

## 3. Update the clean main checkouts

Require no tracked baseline changes, then fast-forward both baselines. Ignore unrelated untracked thoughts data unless it blocks the pull.

```bash
for repo in ../vamos-main "$main_thoughts_repo"; do
  test "$(git -C "$repo" branch --show-current)" = main
  git -C "$repo" diff --quiet
  git -C "$repo" diff --cached --quiet
  git -C "$repo" pull --ff-only origin main
done

test "$(git -C ../vamos rev-parse HEAD)" = "$(git -C ../vamos-main rev-parse HEAD)"
test "$(git -C "$thoughts_repo" rev-parse HEAD)" = "$(git -C "$main_thoughts_repo" rev-parse HEAD)"
```

## 4. Build and restart stage

Record the stage web PID or start time. Build from the working thoughts repo. Then run the host-owned restart command. Include the worker when its build outputs changed. Verify a new web PID or start time and the expected runtime binary path.

```bash
set -euo pipefail
cd "$thoughts_repo"
just build --no-restart
bash -lc "$VAMOS_STAGE_RESTART_CMD"
stage_url=$VAMOS_STAGE_URL
stage_code=$(curl -sS --retry 8 --retry-delay 1 --retry-connrefused \
  -o /dev/null -w '%{http_code}' "$stage_url" -m 20)
case "$stage_code" in 200|301|302|303|307|308) ;; *) exit 1 ;; esac
```

Verify the changed page or asset at the stage public URL. A login redirect proves reachability, not the deployed UI version.

If stage fails, do not rebuild main.

## 5. Build and restart main

**Do not skip this step after the fast-forward.** Record the main web PID or start time. Build from the clean main thoughts baseline. Then run the host-owned restart command. Include the worker when its build outputs changed. Verify a new web PID or start time and the expected runtime binary path.

```bash
set -euo pipefail
cd "$main_thoughts_repo"
just build --no-restart
bash -lc "$VAMOS_MAIN_RESTART_CMD"
main_url=$VAMOS_MAIN_URL
main_code=$(curl -sS --retry 8 --retry-delay 1 --retry-connrefused \
  -o /dev/null -w '%{http_code}' "$main_url" -m 20)
case "$main_code" in 200|301|302|303|307|308) ;; *) exit 1 ;; esac
```

Verify the changed page or asset at the main public URL, not at localhost or stage. Verify the cache key when a JS/CSS asset changed. Do not claim success from an HTTP login redirect alone.

## 6. Report

```bash
for repo in ../datastarui ../vamos ../vamos-main "$thoughts_repo" "$main_thoughts_repo"; do
  printf '%s %s %s\n' \
    "$repo" \
    "$(git -C "$repo" rev-parse --short HEAD)" \
    "$(git -C "$repo" log -1 --format=%s)"
done
printf 'stage: %s HTTP %s\nmain: %s HTTP %s\n' \
  "$stage_url" "$stage_code" "$main_url" "$main_code"
```

Report source sync and deployment separately: repo HEADs, pushes, builds, restarts, new process identity, and both public HTTP results. Include evidence that main serves the changed page or asset. Success requires matching repo HEADs, clean tracked baselines, and verified live deployments. If deployment was explicitly deferred, say **source synced; deployment not performed**.
