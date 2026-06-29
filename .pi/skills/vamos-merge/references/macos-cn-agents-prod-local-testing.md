# macOS cn-agents-prod local site testing after Vamos merge

Use this when a Vamos runtime branch has been fast-forwarded into `../vamos-main` and the user wants to test it on the local/public cn-agents site running on a macOS host.

Observed topology on swarm Mac:

- The public/local site LaunchAgent may be `dev.chestnut.cn-agents`, not the Linux/systemd `vamos` unit.
- The LaunchAgent wrapper may run `/Users/swarm/cn/chestnut-flake/cn-agents-prod/pkg/agents/agents-server` from `cn-agents-prod`, even though `cn-agents/go.mod` points at `../vamos-main` for normal host builds.
- `just build` from `cn-agents` can fail if the newer `dev.vamos-ts-worker` LaunchAgent is not installed, while `just build --no-restart` still succeeds and produces a fresh `../vamos-main/agents-server`.

Safe local-test pattern:

1. Build the runtime/host without relying on LaunchAgent restart hooks:

   ```bash
   cd ~/cn/chestnut-flake/cn-agents
   just build --no-restart
   ```

2. Inspect the active LaunchAgent before touching it:

   ```bash
   launchctl list | grep -E 'dev\.(chestnut\.)?cn-agents|vamos'
   read_file ~/Library/LaunchAgents/dev.chestnut.cn-agents.plist
   read_file "$HOME/Library/Application Support/cn-agents/run-cn-agents-prod-macos.sh"
   ps -p "$(launchctl list | awk '/dev.chestnut.cn-agents$/ {print $1}')" -o pid=,command=
   ```

3. If the active site binary is `cn-agents-prod/pkg/agents/agents-server`, copy the freshly built Vamos binary there with a timestamped backup outside the repo, then restart only that LaunchAgent:

   ```bash
   prod_bin="$HOME/cn/chestnut-flake/cn-agents-prod/pkg/agents/agents-server"
   new_bin="$HOME/cn/chestnut-flake/vamos-main/agents-server"
   backup="$HOME/cn/chestnut-flake/.backups/cn-agents-prod/agents-server.backup-$(date +%Y%m%d-%H%M%S)"
   mkdir -p "$(dirname "$backup")"
   cp "$prod_bin" "$backup"
   cp "$new_bin" "$prod_bin"
   chmod +x "$prod_bin"
   launchctl kickstart -k "gui/$(id -u)/dev.chestnut.cn-agents"
   ```

4. Verify the actual site and public tunnel, not just process state:

   ```bash
   curl -sS -D /tmp/cn-agents-local.headers http://localhost:4200/login -o /tmp/cn-agents-local.html -m 15
   curl -ksS -D /tmp/cn-agents-public.headers https://chestnut-agents-internal.ngrok-free.dev/login -o /tmp/cn-agents-public.html -m 15
   curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' http://localhost:4200/thoughts/tests/filetypes/sample.go -m 10
   ```

5. Keep the repo clean:

   - Put binary backups under `~/cn/chestnut-flake/.backups/`, not inside `cn-agents-prod/pkg/`.
   - Run `git status --short` in `cn-agents-prod` before finishing.

Do not record this as a remote production deployment. It is a local macOS dogfood-site test path for getting a freshly built `../vamos-main` binary onto the running cn-agents site.