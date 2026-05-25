---
name: vamos-merge
description: Merge completed Vamos runtime work into main, sync clean ../vamos-main and ../cn-agents-main baselines, then rebuild/restart the systemd host serving main.workspaces.creative-mode.ai. Use when asked to merge Vamos work, sync Vamos main/baseline, run /vamos-merge, or get latest Vamos running under systemd.
---

# Vamos Merge

Merge a completed `../vamos` branch into `main`, fast-forward the clean baseline checkouts `../vamos-main` and `../cn-agents-main`, then rebuild/restart the clean host checkout so `https://main.workspaces.creative-mode.ai` runs the new runtime.

## Invariants

- Runtime source checkout: `../vamos`.
- Clean baseline checkout: `../vamos-main`.
- Host working checkout: `../cn-agents` owns host changes.
- Host systemd checkout: `../cn-agents-main` is the clean/browser-visible host checkout used for rebuilds and service restarts.
- Browser-visible `../cn-agents-main` imports the clean runtime baseline with `replace github.com/CoreyCole/vamos => ../vamos-main` and launches `../vamos-main/agents-server`.
- `vamos-ts-worker` also runs from `../vamos-main`.
- Fast-forwarding `../vamos-main` does **not** by itself update the running site. Always rebuild from `../cn-agents-main` (`just build --no-restart`) and restart systemd after syncing `../vamos-main` so Go binaries, TS worker output, Tailwind/static assets, and host wrapper all reflect the new commit.
- `../vamos-main` must stay clean; do not edit there. Host rebuilds may generate ignored/build outputs there but must leave `git status` clean.
- `.agents` is a committed symlink in Vamos: `.agents -> ../.agents`. Commit the symlink only, never the target files.
- Systemd is installed/restarted only from clean `../cn-agents-main`, never from `../cn-agents` or a copied workspace.
- Preserve commit shape; do not squash, patch-apply, or cherry-pick the runtime stack.
- Do **not** check out or fetch a copied workspace feature branch into `../vamos` until that feature branch stack has already been synced/restacked onto latest `origin/main` in its own source checkout.
- The sync/restack phase is the same procedure as `.pi/skills/vamos-sync/SKILL.md`; use that skill for conflict handling details.

## Step 1: Preflight

From `../vamos` or a Vamos implementation workspace. Record the starting checkout as the source; later sync/restack happens there.

```bash
pwd
source_checkout=$(git rev-parse --show-toplevel)
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

## Step 2: Publish latest `../vamos/main`, then sync/restack the source branch

First make the canonical working checkout's `main` visible to the remote. This prevents a copied workspace stack from restacking onto stale `origin/main` while `../vamos/main` has newer local trunk commits.

```bash
cd ../vamos
git switch main
git status --short # must be clean
git fetch origin +refs/heads/main:refs/remotes/origin/main
git merge-base --is-ancestor origin/main main
git push origin main
```

Then sync/restack the feature stack in the source checkout where the feature branch already lives. If the source is a copied implementation workspace, run these commands there, not in `../vamos`. If the source is `../vamos`, switch back to the feature branch only after the `main` push above succeeds.

```bash
cd "$source_checkout"
source_branch=$(git branch --show-current)
git status --short # must be clean
git fetch origin +refs/heads/main:refs/remotes/origin/main
gt sync --no-interactive
gt restack --branch "$source_branch" --no-interactive

git status --short
git merge-base --is-ancestor origin/main HEAD
git log --format='%h %s' --reverse origin/main..HEAD
git diff --stat origin/main..HEAD
```

Conflict handling follows `.pi/skills/vamos-sync/SKILL.md`: resolve conflicts in the source checkout, preserve both latest `main` behavior and stack intent, regenerate templ/sqlc/E2E outputs from sources, run targeted tests, continue with `gt continue --no-interactive`, and ask the user to approve conflict resolutions before merging.

Only after this synced/restacked source branch is clean should later steps fetch or fast-forward it into `../vamos`.

## Step 3: Confirm merge preview

Show the user:

```bash
cd "$source_checkout"
printf 'Source checkout: %s\n' "$(pwd)"
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
source_checkout=${source_checkout:-$(pwd)}
source_head=$(git -C "$source_checkout" rev-parse HEAD)
source_branch=$(git -C "$source_checkout" branch --show-current)

cd ../vamos
git switch main
git status --short # must be clean
git fetch origin +refs/heads/main:refs/remotes/origin/main
git merge-base --is-ancestor origin/main main

if test "$(git rev-parse --show-toplevel)" != "$(git -C "$source_checkout" rev-parse --show-toplevel)"; then
  git fetch "$source_checkout" "$source_branch"
  test "$(git rev-parse FETCH_HEAD)" = "$source_head"
  git merge-base --is-ancestor main FETCH_HEAD
  git update-ref refs/heads/main FETCH_HEAD
else
  git merge-base --is-ancestor main "$source_head"
  git update-ref refs/heads/main "$source_head"
fi

git read-tree --reset -u HEAD
test "$(git rev-parse HEAD)" = "$source_head"
```

Never import an unsynced copied-workspace branch into `../vamos` and then try to restack it there. The branch must already have passed Step 2 in its source checkout.

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

Do not edit `../vamos-main`; after host rebuilds, verify it remains clean.

## Step 6: Fast-forward ../cn-agents-main and restart host

Because the browser-visible host imports and launches `../vamos-main`, every merged Vamos runtime change needs `../vamos-main` fast-forwarded first, then a host rebuild from clean `../cn-agents-main`.

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
pid=$(systemctl --user show vamos -p MainPID --value); readlink /proc/$pid/exe
(cd ../vamos-main && git status --short)
```

Expected:

- `vamos` WorkingDirectory is `.../cn-agents-main`.
- `vamos-ts-worker` WorkingDirectory is `.../vamos-main`.
- `vamos` executable is `.../vamos-main/agents-server`.
- `../vamos-main` remains git-clean after build/restart.
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

Print this status block at the end of every successful run:

```bash
printf 'Merge status (local):\n'
for repo in ../vamos ../vamos-main ../cn-agents ../cn-agents-main; do
  printf '%s %s %s\n' \
    "$repo" \
    "$(git -C "$repo" rev-parse --short HEAD)" \
    "$(git -C "$repo" log -1 --format=%s)"
done
printf 'Services: '
systemctl --user is-active temporal-server vamos vamos-ts-worker | paste -sd ' ' -
printf 'vamos exe: '
pid=$(systemctl --user show vamos -p MainPID --value); readlink /proc/$pid/exe
printf 'vamos-ts-worker cwd: '
tspid=$(systemctl --user show vamos-ts-worker -p MainPID --value); readlink /proc/$tspid/cwd
```

Then report:

- final `../vamos` branch/HEAD and latest commit message
- final `../vamos-main` branch/HEAD and latest commit message
- final `../cn-agents` branch/HEAD and latest commit message
- final `../cn-agents-main` branch/HEAD and latest commit message
- systemd active states
- verification URL/result
- push status
