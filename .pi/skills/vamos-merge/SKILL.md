---
name: vamos-merge
description: Merge completed Vamos runtime work into main, sync clean ../vamos-main and ../cn-agents-main baselines, then rebuild/restart the systemd host serving main.workspaces.creative-mode.ai. Use when asked to merge Vamos work, sync Vamos main/baseline, run /vamos-merge, or get latest Vamos running under systemd.
---

# Vamos Merge

Merge a completed `../vamos` branch into `main`, fast-forward the clean baseline checkouts `../vamos-main` and `../cn-agents-main`, then rebuild/restart the clean host checkout so `https://main.workspaces.creative-mode.ai` runs the new runtime.

## Invariants

- Runtime source checkout: `../vamos`.
- Clean baseline checkout: `../vamos-main`.
- Host working checkout: `../cn-agents` owns host changes and imports `github.com/CoreyCole/vamos` with `replace => ../vamos`.
- Host systemd checkout: `../cn-agents-main` is the clean/browser-visible host checkout used for rebuilds and service restarts.
- `../vamos-main` must stay clean; do not edit there.
- `.agents` is a committed symlink in Vamos: `.agents -> ../.agents`. Commit the symlink only, never the target files.
- Systemd is installed/restarted only from clean `../cn-agents-main`, never from `../cn-agents` or a copied workspace.
- Preserve commit shape; do not squash, patch-apply, or cherry-pick the runtime stack.

## Step 1: Preflight

From `../vamos` or a Vamos implementation workspace:

```bash
pwd
git rev-parse --show-toplevel
git branch --show-current
git status --short
(cd ../vamos && git branch --show-current && git status --short)
(cd ../vamos-main && git branch --show-current && git status --short)
(cd ../cn-agents && git branch --show-current && git status --short)
(cd ../cn-agents-main && git branch --show-current && git status --short)
```

Rules:

- Source tracked files must be clean before merge. Stop for dirty tracked files.
- `../vamos-main` must be on `main` and clean. Stop if dirty.
- `../cn-agents` should be on `main`; it is the host working checkout for host changes.
- `../cn-agents-main` must be on `main` and clean except private gitignored config. Stop if tracked files are dirty.
- If `.agents` is missing in Vamos, create `ln -s ../.agents .agents` and commit that symlink in Vamos.

## Step 2: Sync latest main and restack source

```bash
cd ../vamos
source_branch=$(git branch --show-current)
git fetch origin +refs/heads/main:refs/remotes/origin/main

gt sync --no-interactive
gt restack --branch "$source_branch" --no-interactive

git status --short
git merge-base --is-ancestor origin/main HEAD
git log --format='%h %s' --reverse origin/main..HEAD
git diff --stat origin/main..HEAD
```

If conflicts occur, resolve only the Vamos runtime branch conflict, run targeted tests, continue with `gt continue --no-interactive`, then ask the user to approve conflict resolutions before merging.

## Step 3: Confirm merge preview

Show the user:

```bash
cd ../vamos
printf 'Source branch: %s\n' "$(git branch --show-current)"
printf 'Source head: %s\n' "$(git rev-parse HEAD)"
gt log short
printf '\nCommits landing on Vamos main:\n'
git log --format='%h %s' --reverse origin/main..HEAD
printf '\nFiles changed:\n'
git diff --stat origin/main..HEAD
```

Ask: `Proceed with vamos merge? (yes/no)`. Do not continue without explicit yes.

## Step 4: Fast-forward ../vamos main

```bash
cd ../vamos
source_branch=$(git branch --show-current)
source_head=$(git rev-parse HEAD)
git status --short

git merge-base --is-ancestor main "$source_head"
git update-ref refs/heads/main "$source_head"
git switch main
git read-tree --reset -u HEAD
test "$(git rev-parse HEAD)" = "$source_head"
```

If the source branch is not in `../vamos` (for example a copied Vamos workspace), fetch from that absolute path into `../vamos` first, then fast-forward `../vamos/main` to `FETCH_HEAD` only after `git merge-base --is-ancestor main FETCH_HEAD` succeeds.

## Step 5: Fast-forward ../vamos-main baseline

```bash
cd ../vamos-main
git branch --show-current # must be main
git status --short        # must be clean
git fetch ../vamos main
git merge-base --is-ancestor HEAD FETCH_HEAD
git update-ref refs/heads/main FETCH_HEAD
git read-tree --reset -u HEAD
test "$(git rev-parse HEAD)" = "$(git -C ../vamos rev-parse HEAD)"
git status --short
```

Do not run builds or edits in `../vamos-main`.

## Step 6: Fast-forward ../cn-agents-main and restart host

Because the host binary links against `replace github.com/CoreyCole/vamos => ../vamos`, every merged Vamos runtime change needs a host rebuild from clean `../cn-agents-main`.

```bash
cd ../cn-agents-main
git branch --show-current # must be main
git status --short        # no tracked changes; private gitignored config is OK
git fetch ../cn-agents main
git merge-base --is-ancestor HEAD FETCH_HEAD
git update-ref refs/heads/main FETCH_HEAD
git read-tree --reset -u HEAD

just build --no-restart
just install-systemd
systemctl --user daemon-reload
systemctl --user restart temporal-server
systemctl --user restart vamos
systemctl --user restart vamos-ts-worker
sleep 5
systemctl --user is-active temporal-server vamos vamos-ts-worker
systemctl --user show vamos -p WorkingDirectory --no-pager
systemctl --user show vamos-ts-worker -p WorkingDirectory --no-pager
```

Expected:

- `vamos` WorkingDirectory is `.../cn-agents-main`.
- `vamos-ts-worker` WorkingDirectory is `.../vamos`.
- `cn-agents.service`, `cn-agents-ts-worker.service`, and `cn-temporal.service` are inactive after cutover.

## Step 7: Verify browser-visible server

```bash
curl -ksS -D /tmp/vamos-main.headers https://main.workspaces.creative-mode.ai/login \
  -o /tmp/vamos-main-login.html -m 15
head -10 /tmp/vamos-main.headers
rg -n "<title>|<h1" /tmp/vamos-main-login.html

tail -80 ../cn-agents-main/log/vamos.error.log
tail -80 ../cn-agents-main/log/vamos.log
```

Success criteria:

- HTTP 200 or expected auth redirect for the tested route.
- Page content reflects expected app config/feature.
- `vamos.service` remains active after the request.
- No fresh startup errors in `log/vamos.error.log`.

## Step 8: Push policy

Push only after the user agrees:

```bash
cd ../vamos
git push origin main
```

If host-only follow-up fixes were required, commit them on working `../cn-agents/main`, fast-forward `../cn-agents-main`, and push that repo separately after approval.

## Completion response

Report:

- final `../vamos` branch/HEAD
- final `../vamos-main` branch/HEAD
- final `../cn-agents-main` branch/HEAD if rebuilt
- systemd active states
- verification URL/result
- push status
