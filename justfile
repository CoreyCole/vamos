build *args:
  @go run ./cmd/build-agents {{args}}

# Regenerate derived Go code, lint, and run the unit test suite.
check:
  @templ generate
  @sqlc generate
  @golangci-lint run
  @go test ./...

e2e *args:
  @VAMOS_ROOT="$PWD"; ../datastarui/scripts/datastarui.sh e2e run --config "$VAMOS_ROOT/datastarui-e2e.yml" {{args}}

sync-thoughts:
  @cd .. && just sync-thoughts

verify-workspace-db db=".vamos/run/agents.db" format="text":
  @set -eu; \
    db="{{db}}"; db="${db#db=}"; \
    format="{{format}}"; format="${format#format=}"; \
    scripts/workspace-db-verify/verify.sh --database-path "$db" --format "$format"

verify-workspaces slug start="true" restart="true" stop="true" browser="false" agent_chat_probe="false" report="" remote="" dns_server="" expect_ip="" require_remote="false":
  @set -eu; \
    slug="{{slug}}"; slug="${slug#slug=}"; \
    start="{{start}}"; start="${start#start=}"; \
    restart="{{restart}}"; restart="${restart#restart=}"; \
    stop="{{stop}}"; stop="${stop#stop=}"; \
    browser="{{browser}}"; browser="${browser#browser=}"; \
    agent_chat_probe="{{agent_chat_probe}}"; agent_chat_probe="${agent_chat_probe#agent_chat_probe=}"; \
    report="{{report}}"; report="${report#report=}"; \
    remote="{{remote}}"; remote="${remote#remote=}"; \
    dns_server="{{dns_server}}"; dns_server="${dns_server#dns_server=}"; \
    expect_ip="{{expect_ip}}"; expect_ip="${expect_ip#expect_ip=}"; \
    require_remote="{{require_remote}}"; require_remote="${require_remote#require_remote=}"; \
    args="--env .env --slug $slug --start=$start --restart=$restart --stop=$stop --browser=$browser --agent-chat-probe=$agent_chat_probe"; \
    if [ -n "$report" ]; then args="$args --report $report"; fi; \
    if [ -n "$remote" ]; then args="$args --remote-ssh $remote"; fi; \
    if [ -n "$dns_server" ]; then args="$args --dns-server $dns_server"; fi; \
    if [ -n "$expect_ip" ]; then args="$args --expect-ip $expect_ip"; fi; \
    if [ "$require_remote" = "true" ]; then args="$args --require-remote-tailnet"; fi; \
    go run ./cmd/vamos-runtime ctl verify workspaces $args

# Start an isolated local manager for browser verification on a fixed loopback URL.
up-veriy oauth_file="":
  @set -eu; \
    oauth_file="{{oauth_file}}"; oauth_file="${oauth_file#oauth_file=}"; \
    state_dir="$PWD/.vamos/verify"; pid_file="$state_dir/server.pid"; \
    log_file="$state_dir/server.log"; server_bin="$state_dir/vamos-verify-server"; \
    url="http://127.0.0.1:49231"; \
    mkdir -p "$state_dir"; \
    if [ ! -d "$PWD/thoughts" ]; then echo "Expected thoughts root is missing: $PWD/thoughts" >&2; exit 2; fi; \
    if [ -z "$oauth_file" ]; then oauth_file="${GOOGLE_CREDENTIALS_FILE:-}"; fi; \
    if [ -z "$oauth_file" ]; then \
      oauth_file="$state_dir/local-google-oauth-client.json"; \
      if [ ! -f "$oauth_file" ]; then \
        umask 077; \
        printf '%s\n' '{"web":{"client_id":"local-verification","client_secret":"local-verification","redirect_uris":["http://127.0.0.1:49231/auth/callback"]}}' > "$oauth_file"; \
      fi; \
    fi; \
    if [ ! -f "$oauth_file" ]; then \
      echo "OAuth client file not found: $oauth_file" >&2; \
      exit 2; \
    fi; \
    if [ -s "$pid_file" ] && kill -0 "$(cat "$pid_file")" 2>/dev/null; then \
      echo "Verification server already running at $url (pid $(cat "$pid_file"))."; exit 0; \
    fi; \
    rm -f "$pid_file"; \
    go build -o "$server_bin" ./cmd/server; \
    signing_key="$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=')"; internal_token="$(openssl rand -hex 32)"; \
    ( exec env \
      VAMOS_LISTEN_ADDRESS="127.0.0.1:49231" \
      VAMOS_PUBLIC_BASE_URL="$url" \
      VAMOS_DATABASE_PATH="$PWD/.vamos/state/agents.db" \
      VAMOS_THOUGHTS_REPO="$PWD" \
      VAMOS_THOUGHTS_ROOT="$PWD/thoughts" \
      VAMOS_WORKSPACE_MODE="manager" \
      VAMOS_WORKSPACE_DOMAIN="localhost" \
      VAMOS_WORKSPACE_SLUG="main" \
      "$(printf '%s=%s' VAMOS_DEV_AUTH_SIGNING_KEY "$signing_key")" \
      VAMOS_PLAYWRIGHT_AUTH_ENABLED="true" \
      AUTH_WHITELISTED_EMAILS="playwright@localhost" \
      "$(printf '%s=%s' GOOGLE_CREDENTIALS_FILE "$oauth_file")" \
      "$(printf '%s=%s' VAMOS_INTERNAL_TOKEN "$internal_token")" \
      VAMOS_INTERNAL_ALLOW_LOOPBACK="true" \
      "$server_bin" >"$log_file" 2>&1 ) & \
    pid="$!"; printf '%s\n' "$pid" > "$pid_file"; ready=""; \
    for _ in $(seq 1 60); do \
      if ! kill -0 "$pid" 2>/dev/null; then cat "$log_file" >&2; rm -f "$pid_file"; exit 1; fi; \
      status="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 1 "$url/" || true)"; \
      if [ "$status" = "200" ] || [ "$status" = "307" ]; then ready="$status"; break; fi; \
      sleep 1; \
    done; \
    if [ -z "$ready" ]; then echo "Verification server did not become ready; see $log_file" >&2; exit 1; fi; \
    if ! go run ./cmd/vamos-runtime auth status --manager-url "$url" --slug main >/dev/null 2>&1; then \
      go run ./cmd/vamos-runtime auth create-machine-key \
        --database-path "$PWD/.vamos/state/agents.db" --manager-url "$url" \
        --name local-e2e --email playwright@localhost --slug main \
        --purpose e2e_playwright --purpose verify --save-profile >/dev/null; \
    fi; \
    echo "Verification server ready: $url (HTTP $ready)"; \
    echo "Server log: $log_file"; echo ""; \
    echo "For Playwright CLI, mint an ephemeral token in the same shell:"; \
    echo '  eval "$(go run ./cmd/vamos-runtime auth playwright-env --manager-url http://127.0.0.1:49231 --slug main)"'; \
    echo '  playwright-cli open "http://127.0.0.1:49231/internal/agent-auth/browser-login?purpose=e2e_playwright&token=$VAMOS_E2E_AUTH_TOKEN&redirect=/"'; \
    echo ""; echo "For authored Go Stories:"; \
    echo '  VAMOS_E2E_THOUGHTS_ROOT="$PWD/thoughts" just e2e --base-url http://127.0.0.1:49231 --no-restart --story static-html-applet-embedded'

down-verify:
  @set -eu; \
    pid_file="$PWD/.vamos/verify/server.pid"; \
    if [ ! -s "$pid_file" ]; then echo "Verification server is not running."; exit 0; fi; \
    pid="$(cat "$pid_file")"; \
    if kill -0 "$pid" 2>/dev/null; then \
      kill "$pid"; \
      for _ in $(seq 1 10); do if ! kill -0 "$pid" 2>/dev/null; then break; fi; sleep 1; done; \
      if kill -0 "$pid" 2>/dev/null; then kill -9 "$pid"; fi; \
    fi; \
    rm -f "$pid_file"; echo "Verification server stopped."
