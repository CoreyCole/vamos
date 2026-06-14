build *args:
  @go run ./cmd/build-agents {{args}}

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
