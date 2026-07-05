---
name: vamos-merge
description: Merge completed Vamos runtime work into main, including any paired DatastarUI stack that Vamos depends on, verify the durable stage lane is operational from the merged ../vamos checkout, then sync clean ../vamos-main and ../cn-agents-main baselines and rebuild/restart the systemd host serving main.workspaces.creative-mode.ai. Use when asked to merge Vamos work, sync Vamos main/baseline, run /vamos-merge, merge a Vamos+DatastarUI stack, or get latest Vamos running under systemd.
---

# Vamos Merge

Merge a completed `../vamos` branch into `main` plus any paired `../datastarui` stack that the Vamos branch depends on, verify the durable `stage` lane is healthy from the merged local runtime checkout, then fast-forward the clean baseline checkouts `../vamos-main` and `../cn-agents-main` and rebuild/restart the clean host checkout so `https://main.workspaces.creative-mode.ai` runs the new runtime.

## Invariants

- Runtime source checkout: `../vamos`.
- Durable stage lane checkout: `../vamos` in this host setup.
- Clean baseline checkout: `../vamos-main`.
- Host working checkout: `../cn-agents` owns host changes and is where any tracked host edits must be committed before merge.
- Host systemd checkout: `../cn-agents-main` is the clean/browser-visible host checkout used for rebuilds and service restarts. Do not make normal tracked edits there.
- Optional paired DatastarUI checkout: `../datastarui`. When the Vamos stack depends on an unmerged DatastarUI stack, merge/push the DatastarUI stack first, then make the Vamos stack consume a merged/resolvable DatastarUI state (version, commit, copied source, or approved replace) before merging Vamos.
- Browser-visible `../cn-agents-main` imports the clean runtime baseline with `replace github.com/CoreyCole/vamos => ../vamos-main` and launches `../vamos-main/agents-server`.
- `vamos-ts-worker` also runs from `../vamos-main`.
- Before fast-forwarding `../vamos-main` or pushing merged runtime commits to `origin/main`, prove the durable `stage` lane can run the merged `../vamos` commit. If stage verification fails, stop the merge flow and fix stage first.
- Fast-forwarding `../vamos-main` does **not** by itself update the running site. Always rebuild from `../cn-agents-main` (`just build --no-restart`) and restart systemd after syncing `../vamos-main` so Go binaries, TS worker output, Tailwind/static assets, and host wrapper all reflect the new commit.
- `../vamos-main` must stay clean; do not edit there. Host rebuilds may generate ignored/build outputs there but must leave `git status` clean.
- `.agents` is a committed symlink in Vamos: `.agents -> ../.agents`. Commit the symlink only, never the target files.
- Systemd is installed/restarted only from clean `../cn-agents-main`, never from `../cn-agents` or a copied workspace.
- Preserve commit shape; do not squash, patch-apply, or cherry-pick the runtime stack.
- Preserve paired DatastarUI stack shape too. If DatastarUI changes are part of the work, do not squash or patch-copy them into Vamos. Merge/push DatastarUI through its own Graphite stack, then update/verify Vamos against the merged DatastarUI result.
- Do **not** check out or fetch a copied workspace feature branch into `../vamos` until that feature branch stack has already been synced/restacked onto latest `origin/main` in its own source checkout.
- The sync/restack phase is the same procedure as `.pi/skills/vamos-sync/SKILL.md`; use that skill for conflict handling details.
- After pushing merged Vamos runtime changes to remote, update the dotfiles/context Vamos skill source checkout (`~/dotfiles/context/vamos`) so future Pi agents load the latest committed `.pi` skills, prompts, and q-manager guidance. This is especially important when the merge changes `.pi/skills/*`.

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
if test -d ../datastarui; then (cd ../datastarui && git branch --show-current && git status --short); fi
```

Rules:

- Source tracked files must be clean before merge. If the source checkout has task-owned pending changes, commit them first; if the changes are unrelated or ambiguous, stop and triage instead of merging them implicitly.
- `../vamos` is the durable stage lane checkout in this host setup and should be on `main` with clean tracked files before merge. `.vamos/` runtime state may be gitignored.
- `../vamos-main` must be on `main` and clean. Stop if dirty.
- `../cn-agents` should be on `main`; it is the host working checkout for host changes. If host-side changes are part of this merge flow, commit them in `../cn-agents` before syncing baselines. Do not leave task-owned tracked changes pending there.
- `../cn-agents-main` must be on `main` and clean except private gitignored config. Stop if tracked files are dirty. If you accidentally edited tracked files there, move or re-apply those edits in `../cn-agents`, then restore `../cn-agents-main` to clean before continuing.
- If `../datastarui` is present and on a non-main branch with commits ahead of `main`, determine whether the Vamos source branch depends on it. Check `go.mod` replaces, copied-source manifests/locks, imports, plan/handoff notes, and user intent. If dependent, treat `../datastarui` as a required paired stack and merge it before merging Vamos. If unrelated or ambiguous, stop and ask.
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

Conflict handling follows `.pi/skills/vamos-sync/SKILL.md`: resolve conflicts in the source checkout, preserve both latest `main` behavior and stack intent, regenerate templ/sqlc/E2E outputs from sources, run targeted tests, stage resolved files with `gt add <file>` (or `git add` only when Graphite has no wrapper for that file), then continue with **`gt continue --no-interactive`**. Do **not** use `git rebase --continue` for Graphite restack conflicts; it may bypass Graphite bookkeeping or open an editor. Ask the user to approve conflict resolutions before merging.

Only after this synced/restacked source branch is clean should later steps fetch or fast-forward it into `../vamos`.

## Step 2.1: Merge paired DatastarUI stack first when required

Run this step when the Vamos source branch depends on `../datastarui` changes that are not on DatastarUI `main` yet. Examples: `go.mod` has `replace github.com/coreycole/datastarui => ../datastarui`, Vamos copied-source/lock files were generated from the DatastarUI branch, or the plan/handoff says the work spans both repos.

```bash
if test -d ../datastarui; then
  cd ../datastarui
  datastarui_branch=$(git branch --show-current)
  git status --short # must be clean before merge; commit task-owned changes first
  git fetch origin +refs/heads/main:refs/remotes/origin/main
  gt sync --no-interactive
  gt restack --branch "$datastarui_branch" --no-interactive
  git status --short
  git merge-base --is-ancestor origin/main HEAD
  git log --format='%h %s' --reverse origin/main..HEAD
  git diff --stat origin/main..HEAD
fi
```

If the DatastarUI stack is required and synced/restacked cleanly, fast-forward DatastarUI `main` and push it before merging Vamos:

```bash
cd ../datastarui
datastarui_head=$(git rev-parse HEAD)
datastarui_branch=$(git branch --show-current)
git switch main
git status --short # must be clean
git fetch origin +refs/heads/main:refs/remotes/origin/main
git merge-base --is-ancestor origin/main main
git fetch . "$datastarui_branch"
git merge-base --is-ancestor main "$datastarui_head"
git update-ref refs/heads/main "$datastarui_head"
git read-tree --reset -u HEAD
git push origin main
```

Then return to the Vamos source checkout and make the Vamos dependency state merge-safe before continuing:

- If Vamos temporarily used `replace github.com/coreycole/datastarui => ../datastarui`, remove it or replace it with an approved merge-safe dependency reference before merging Vamos, unless the repo explicitly intends to keep a sibling replace.
- If Vamos uses copied DatastarUI source, regenerate/update the copied source from the now-merged DatastarUI `main` and commit that in the Vamos source stack.
- Run the relevant Vamos tests/builds that prove the merged DatastarUI dependency still satisfies the Vamos branch.
- If DatastarUI merge fails, stop. Do not merge Vamos while it depends on an unmerged DatastarUI branch.

Skip this step only when DatastarUI is not part of the current work or the user explicitly confirms the DatastarUI stack has already merged and Vamos no longer depends on unmerged DatastarUI commits.

## Step 2.5: Commit pending task-owned changes in ../vamos and ../cn-agents

Before any baseline fast-forward or restart work, ensure task-owned tracked changes are committed in the writable working checkouts.

- If `../vamos` still has pending tracked changes for the merge task, stage only those files and commit them on the correct branch before continuing.
- If `../cn-agents` has pending tracked host/config/doc changes that are part of the same rollout, stage only those files and commit them in `../cn-agents` before continuing.
- Do not commit unrelated dirty files from other agents or half-finished work. Stop and ask if ownership is unclear.
- Never make these commits in `../cn-agents-main`; that checkout must stay clean and browser-visible only.

## Step 3: Merge preview

Print the merge preview for auditability, then continue. Do not ask for confirmation: invoking `/vamos-merge` means the user is ready to fast-forward, deploy, and push unless a preflight, sync/restack, verification, or safety check fails.

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

## Step 5: Verify the durable stage lane from ../vamos

Before touching `../vamos-main` or pushing merged runtime commits to `origin/main`, prove the durable stage lane can boot from the merged `../vamos` checkout.

```bash
cd ../vamos
git branch --show-current # must be main
git status --short        # tracked files must be clean; .vamos runtime state is gitignored

stage_log="log/vamos.log"
stage_error_log="log/vamos.error.log"
if ! test -f "$stage_log" && test -f ".vamos/log/web.log"; then stage_log=".vamos/log/web.log"; fi
if ! test -f "$stage_error_log" && test -f ".vamos/log/web.error.log"; then stage_error_log=".vamos/log/web.error.log"; fi
stage_log_start=$(wc -l <"$stage_log" 2>/dev/null || echo 0)
stage_error_log_start=$(wc -l <"$stage_error_log" 2>/dev/null || echo 0)

just build
sleep 5
curl -ksS -D /tmp/vamos-stage.headers https://stage.workspaces.creative-mode.ai/login \
  -o /tmp/vamos-stage-login.html -m 20
head -10 /tmp/vamos-stage.headers
rg -n "<title>|<h1" /tmp/vamos-stage-login.html

stage_db=".vamos/run/agents.db"
if ! test -s "$stage_db" && test -s ".vamos/state/agents.db"; then stage_db=".vamos/state/agents.db"; fi
test -s "$stage_db"
scripts/workspace-db-verify/verify.sh --database-path "$stage_db" --format text

# Prefer an explicit workspace sync refresh over sleeping for the schedule when the host exposes manager routes.
# Child/read-only workspace hosts may not register POST /workspaces/refresh; in that case poll the fresh log window.
if curl -ksS -o /tmp/vamos-stage-refresh.out -w '%{http_code}' -X POST \
  https://stage.workspaces.creative-mode.ai/workspaces/refresh | rg -q '^(202|303)$'; then
  echo "triggered stage workspace refresh"
else
  echo "stage workspace refresh route unavailable or unauthenticated; trying Temporal schedule trigger"
  temporal_address="127.0.0.1:$(jq -r '.ports.temporal // empty' .vamos/run/status.json 2>/dev/null)"
  if test "$temporal_address" != "127.0.0.1:"; then
    temporal --address "$temporal_address" schedule list > /tmp/vamos-stage-schedules.txt
    schedule_id=$(awk '/SyncCoordinatorWorkflow/ {print $1; exit}' /tmp/vamos-stage-schedules.txt)
    if test -n "$schedule_id"; then
      awk '/SyncWorkspacesWorkflow/ {print $1}' /tmp/vamos-stage-schedules.txt | while read -r legacy_schedule_id; do
        test -n "$legacy_schedule_id" && printf 'y\n' | temporal --address "$temporal_address" schedule delete --schedule-id "$legacy_schedule_id"
      done
    else
      schedule_id=$(awk '/agent-chat-sync-workspaces:/ {print $1; exit}' /tmp/vamos-stage-schedules.txt)
    fi
    if test -n "$schedule_id"; then
      temporal --address "$temporal_address" schedule trigger --schedule-id "$schedule_id" --overlap-policy BufferOne
    fi
  fi
fi

stage_fresh=/tmp/vamos-stage-fresh.log
for i in $(seq 1 24); do
  { tail -n +$((stage_log_start + 1)) "$stage_log" 2>/dev/null || true; tail -n +$((stage_error_log_start + 1)) "$stage_error_log" 2>/dev/null || true; } | tee "$stage_fresh" >/dev/null
  if rg -n "workspace_sync_refresh_complete|workspace sync.*complete|SyncWorkspaces" "$stage_fresh" >/dev/null; then
    break
  fi
  sleep 5
done
rg -n "workspace_sync_refresh_complete|workspace sync.*complete|SyncWorkspaces" "$stage_fresh"
if rg -n "FOREIGN KEY constraint failed|UNIQUE constraint failed" "$stage_fresh"; then
  echo "hard workspace DB constraint failure in fresh stage logs" >&2
  exit 1
fi
if rg -n "SQLITE_BUSY|database is locked" "$stage_fresh"; then
  rg -n "workspace_sync_refresh_complete|workspace sync.*complete|SyncWorkspaces" "$stage_fresh" >/dev/null
fi
```

Success criteria:

- `../vamos` is the durable stage lane checkout for this host setup.
- `just build` succeeds and the workspace restart/start hook does not fail.
- `https://stage.workspaces.creative-mode.ai/login` returns HTTP 200 or expected auth redirect, **not** 503.
- Stage `.vamos/run/agents.db` passes `scripts/workspace-db-verify/verify.sh`.
- Fresh stage logs after the build/restart window contain workspace sync success evidence.
- Fresh `FOREIGN KEY constraint failed` or `UNIQUE constraint failed` lines block immediately.
- Fresh `SQLITE_BUSY` / `database is locked` lines are tolerated only when followed by workspace sync success in the same fresh window.
- If the authenticated main manager switches to stage while stage is already running, the redirect should be immediate; if stage was stopped, the manager should auto-start it and then redirect. If stage cannot start, the main manager should route to `/workspaces/errors?workspace=stage` instead of a dead stage host.

If any stage verification step fails, stop here. Do **not** fast-forward `../vamos-main` and do **not** push merged runtime commits to `origin/main` until stage is healthy.

## Step 6: Fast-forward ../vamos-main baseline

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

## Step 7: Fast-forward host checkout and restart host

Because the browser-visible host imports and launches `../vamos-main`, every merged Vamos runtime change needs `../vamos-main` fast-forwarded first, then the active host rebuilt/restarted.

### Linux/systemd clean-host topology

Use this when `../cn-agents-main` exists and systemd is the active host topology:

```bash
cd ../cn-agents-main
git branch --show-current # must be main
git status --short        # no tracked changes; private gitignored config is OK
git fetch ../cn-agents main
git merge-base --is-ancestor HEAD FETCH_HEAD
git update-ref refs/heads/main FETCH_HEAD
git read-tree --reset -u HEAD

main_log="log/vamos.log"
main_error_log="log/vamos.error.log"
if ! test -f "$main_log" && test -f ".vamos/log/web.log"; then main_log=".vamos/log/web.log"; fi
if ! test -f "$main_error_log" && test -f ".vamos/log/web.error.log"; then main_error_log=".vamos/log/web.error.log"; fi
main_log_start=$(wc -l <"$main_log" 2>/dev/null || echo 0)
main_error_log_start=$(wc -l <"$main_error_log" 2>/dev/null || echo 0)

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

### macOS LaunchAgent / cn-agents-prod test topology

Use this when `../cn-agents-main` is absent and the active browser-visible site is a LaunchAgent such as `dev.chestnut.cn-agents` running a wrapper under `~/Library/Application Support/cn-agents/` with `REPO_DIR=.../cn-agents-prod`. See `references/macos-cn-agents-prod-local-testing.md` for the detailed local-test recipe and backup/verification commands.

### macOS direct-runtime hotfix checklist

When a Vamos runtime hotfix is developed directly in `../vamos/main` for immediate cn-agents-prod testing, use the normal macOS topology but keep these extra guards:

1. Commit task-owned runtime changes in `../vamos` before syncing `../vamos-main`; do not leave the hotfix only as a dirty working tree.

1. If `git push origin main` is rejected as non-fast-forward, fetch/rebase onto `origin/main`, rerun targeted tests, push again, then re-sync `../vamos-main` from the rebased `../vamos` head. A successful pre-rebase prod restart is not enough because the final commit SHA changed.

1. After every re-sync of `../vamos-main`, rebuild from the host wrapper checkout and restart the LaunchAgent again:

   ```bash
   cd ../cn-agents-prod
   just build --no-restart
   launchctl kickstart -k gui/$(id -u)/dev.chestnut.cn-agents
   ```

1. Verify both the active process and the feature behavior:

   ```bash
   ps -p $(launchctl list | awk '/dev.chestnut.cn-agents$/ {print $1}') -o pid=,command=
   curl -fsS http://127.0.0.1:4200/manifest.json >/dev/null
   ```

   For Datastar UI fixes, also run a browser-console check against `localhost:4200`/the ngrok target for the exact JS error being fixed, not just the HTTP health check.

1. Verify the active process before changing anything:

   ```bash
   launchctl list | grep -E 'dev\.chestnut\.cn-agents($|-ts-worker|-ngrok)'
   ps -p $(launchctl list | awk '/dev.chestnut.cn-agents$/ {print $1}') -o pid=,command=
   ```

1. Build `../vamos-main` via the host wrapper checkout. If plain `just build` fails only because a non-active `dev.vamos-ts-worker` LaunchAgent is missing, run `just build --no-restart` and record that restart was manual.

1. If the active LaunchAgent executes `.../cn-agents-prod/pkg/agents/agents-server`, copy the freshly built `../vamos-main/agents-server` to that exact binary path, preserving a backup outside git if needed.

1. Restart with launchd, not systemd:

   ```bash
   launchctl kickstart -k gui/$(id -u)/dev.chestnut.cn-agents
   ```

1. Verify local and public URLs return login/redirect responses, and verify the test paths the user needs. Do not claim browser-visible testing until the active process command points at the updated binary and `curl` confirms the site is reachable.

## Step 8: Verify browser-visible server

```bash
curl -ksS -D /tmp/vamos-main.headers https://main.workspaces.creative-mode.ai/login \
  -o /tmp/vamos-main-login.html -m 15
head -10 /tmp/vamos-main.headers
rg -n "<title>|<h1" /tmp/vamos-main-login.html

main_db=".vamos/run/agents.db"
if ! test -s "$main_db" && test -s ".vamos/state/agents.db"; then main_db=".vamos/state/agents.db"; fi
test -s "$main_db"
../vamos-main/scripts/workspace-db-verify/verify.sh --database-path "$main_db" --format text

# Prefer an explicit workspace sync refresh over sleeping for the schedule when authenticated.
# If this returns 401/403, authenticate in a browser and click Workspaces → Refresh, or mint a main-scoped browser token.
if curl -ksS -o /tmp/vamos-main-refresh.out -w '%{http_code}' -X POST \
  https://main.workspaces.creative-mode.ai/workspaces/refresh | rg -q '^(202|303)$'; then
  echo "triggered main workspace refresh"
else
  echo "main workspace refresh route unavailable or unauthenticated; trying Temporal schedule trigger"
  temporal_address="127.0.0.1:$(jq -r '.ports.temporal // empty' .vamos/run/status.json 2>/dev/null)"
  if test "$temporal_address" = "127.0.0.1:"; then
    temporal_address="127.0.0.1:7233"
  fi
  temporal --address "$temporal_address" schedule list > /tmp/vamos-main-schedules.txt
  schedule_id=$(awk '/SyncCoordinatorWorkflow/ {print $1; exit}' /tmp/vamos-main-schedules.txt)
  if test -n "$schedule_id"; then
    awk '/SyncWorkspacesWorkflow/ {print $1}' /tmp/vamos-main-schedules.txt | while read -r legacy_schedule_id; do
      test -n "$legacy_schedule_id" && printf 'y\n' | temporal --address "$temporal_address" schedule delete --schedule-id "$legacy_schedule_id"
    done
  else
    schedule_id=$(awk '/agent-chat-sync-workspaces:/ {print $1; exit}' /tmp/vamos-main-schedules.txt)
  fi
  if test -n "$schedule_id"; then
    temporal --address "$temporal_address" schedule trigger --schedule-id "$schedule_id" --overlap-policy BufferOne
  fi
fi

main_fresh=/tmp/vamos-main-fresh.log
for i in $(seq 1 24); do
  { tail -n +$((main_log_start + 1)) "$main_log" 2>/dev/null || true; tail -n +$((main_error_log_start + 1)) "$main_error_log" 2>/dev/null || true; } | tee "$main_fresh" >/dev/null
  if rg -n "workspace_sync_refresh_complete|workspace sync.*complete|SyncWorkspaces" "$main_fresh" >/dev/null; then
    break
  fi
  sleep 5
done
rg -n "workspace_sync_refresh_complete|workspace sync.*complete|SyncWorkspaces" "$main_fresh"
if rg -n "FOREIGN KEY constraint failed|UNIQUE constraint failed" "$main_fresh"; then
  echo "hard workspace DB constraint failure in fresh main logs" >&2
  exit 1
fi
if rg -n "SQLITE_BUSY|database is locked" "$main_fresh"; then
  rg -n "workspace_sync_refresh_complete|workspace sync.*complete|SyncWorkspaces" "$main_fresh" >/dev/null
fi

tail -80 "$main_error_log"
tail -80 "$main_log"
```

Success criteria:

- HTTP 200 or expected auth redirect for the tested route.
- Page content reflects expected app config/feature.
- `vamos.service` remains active after the request.
- Browser-visible main `.vamos/run/agents.db` passes `../vamos-main/scripts/workspace-db-verify/verify.sh`.
- Fresh main logs after the rebuild/restart window contain workspace sync success evidence.
- Fresh `FOREIGN KEY constraint failed` or `UNIQUE constraint failed` lines block immediately.
- Fresh `SQLITE_BUSY` / `database is locked` lines are tolerated only when followed by workspace sync success in the same fresh window.
- No fresh startup errors in `log/vamos.error.log`.

## Step 9: Push

Push the merged runtime main automatically, but only after stage verification and browser-visible main verification have both passed. Do not ask for confirmation: `/vamos-merge` is only invoked when the stack is ready to merge and publish.

```bash
cd ../vamos
git push origin main
```

Then sync the dotfiles/context Vamos checkout that Pi uses for durable skill context:

```bash
if test -d ~/dotfiles/context/vamos/.git; then
  cd ~/dotfiles/context/vamos
  git fetch origin +refs/heads/main:refs/remotes/origin/main
  git switch main
  git status --short # must be clean before pull/reset; stop if dirty
  git pull --ff-only origin main
fi
```

If `~/dotfiles/context/vamos` is dirty, stop before overwriting it. Inspect whether the dirty files are task-owned skill/runbook updates from the same session. If they are and the user authorizes committing them, stage only those skill files/support references, commit with a docs/runbook message, rebase/pull onto `origin/main`, push, and verify `HEAD == origin/main`. If ownership is unclear, ask rather than stashing or discarding. Do not leave it stale after a successful remote push, or subsequent Pi agents may load old q-manager/vamos skill instructions.

If host-only follow-up fixes were required, commit them on working `../cn-agents/main`, fast-forward `../cn-agents-main`, and push that repo separately as part of the same merge flow after its verification passes.

## Completion response

Print this status block at the end of every successful run:

```bash
printf 'Merge status (local):\n'
for repo in ../datastarui ../vamos ../vamos-main ../cn-agents ../cn-agents-main; do
  test -d "$repo/.git" || continue
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

- final `../datastarui` branch/HEAD and latest commit message when DatastarUI participated in the merge
- final `../vamos` branch/HEAD and latest commit message
- final `../vamos-main` branch/HEAD and latest commit message
- final `../cn-agents` branch/HEAD and latest commit message
- final `../cn-agents-main` branch/HEAD and latest commit message
- systemd active states
- stage verification URL/result
- verification URL/result
- push status for DatastarUI when applicable, and Vamos
- `~/dotfiles/context/vamos` sync status/HEAD after pull
