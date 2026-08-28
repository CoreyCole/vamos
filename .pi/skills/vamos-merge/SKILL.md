---
name: vamos-merge
description: Land completed Vamos and paired DatastarUI work, refresh Vamos copied UI source, update local stage/main checkouts, then rebuild both dogfood lanes. Use for /vamos-merge, merging Vamos work, syncing local baselines, or rebuilding stage and main.
---

# Vamos Merge

Keep DatastarUI and the local stage/main pairs current, built, and running:

| Role | Source/config checkout | Runtime/copied checkout |
| --- | --- | --- |
| UI source | `../datastarui` | `../vamos/pkg/datastarui` |
| Stage | thoughts repo | `../vamos` |
| Main | main thoughts baseline | `../vamos-main` |

Run from the canonical `../vamos` checkout. Resolve host-owned paths and URLs from the deployment configuration or operator input; do not infer repository names, service names, ports, or domains.

```bash
: "${VAMOS_THOUGHTS_REPO_CHECKOUT:?set the working thoughts repo checkout}"
: "${VAMOS_MAIN_THOUGHTS_REPO_CHECKOUT:?set its clean main baseline checkout}"
: "${VAMOS_STAGE_URL:?set the stage health-check URL}"
: "${VAMOS_MAIN_URL:?set the main health-check URL}"
thoughts_repo=$VAMOS_THOUGHTS_REPO_CHECKOUT
main_thoughts_repo=$VAMOS_MAIN_THOUGHTS_REPO_CHECKOUT
```

Keep these variables for the workflow. In the current dogfood setup, `cn-agents` is the thoughts repo and its `-main` sibling is the clean baseline; other hosts supply equivalent checkouts. Invoking `/vamos-merge` authorizes commits for task-owned changes, fast-forward merges, pushes, builds, and restarts.

## Rules

- Commit only task-owned changes. Stop on unrelated or ambiguous changes.
- All local checkouts must finish on `main` and up to date with `origin/main`.
- Treat `../vamos-main` and `$main_thoughts_repo` as clean baselines. Do not edit them directly.
- Preserve feature commits in Vamos and DatastarUI. Fast-forward only; do not squash, cherry-pick, or create merge commits.
- Update `pkg/datastarui` through the DatastarUI CLI; do not hand-copy or hand-customize copied components.
- Stop on conflicts, non-fast-forward updates, build failures, restart failures, or failed HTTP smoke checks.
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

Build from the working thoughts repo, then run its host-owned restart command for the stage runtime and worker. Read that command from the repo’s deployment docs/config; do not invent service-manager labels.

```bash
cd "$thoughts_repo"
just build --no-restart
stage_url=$VAMOS_STAGE_URL
stage_code=$(curl -ksS -o /dev/null -w '%{http_code}' "$stage_url" -m 20)
case "$stage_code" in 200|301|302|303|307|308) ;; *) exit 1 ;; esac
```

Do not rebuild main if stage fails.

## 5. Build and restart main

Build from the clean main thoughts baseline, then run its host-owned restart command for the main runtime and worker.

```bash
cd "$main_thoughts_repo"
just build --no-restart
main_url=$VAMOS_MAIN_URL
main_code=$(curl -ksS -o /dev/null -w '%{http_code}' "$main_url" -m 20)
case "$main_code" in 200|301|302|303|307|308) ;; *) exit 1 ;; esac
```

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

Report the DatastarUI HEAD, the four stage/main HEADs, copied-source update result, pushes, builds/restarts, and both HTTP results. Success requires a clean DatastarUI copy, matching stage/main repo HEADs, and clean tracked baseline files.
