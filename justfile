build *args:
  @go run ./cmd/build-agents {{args}}

playwright-auth url="http://localhost:4200/agent-chat":
  @scripts/playwright-auth.sh {{url}}

pw-agent-chat:
  @just playwright-auth http://localhost:4200/agent-chat

verify-workspaces slug start="true" restart="true" stop="true" browser="false" report="" remote="" dns_server="" expect_ip="" require_remote="false":
  @set -eu; \
    slug="{{slug}}"; slug="${slug#slug=}"; \
    start="{{start}}"; start="${start#start=}"; \
    restart="{{restart}}"; restart="${restart#restart=}"; \
    stop="{{stop}}"; stop="${stop#stop=}"; \
    browser="{{browser}}"; browser="${browser#browser=}"; \
    report="{{report}}"; report="${report#report=}"; \
    remote="{{remote}}"; remote="${remote#remote=}"; \
    dns_server="{{dns_server}}"; dns_server="${dns_server#dns_server=}"; \
    expect_ip="{{expect_ip}}"; expect_ip="${expect_ip#expect_ip=}"; \
    require_remote="{{require_remote}}"; require_remote="${require_remote#require_remote=}"; \
    args="--env .env --slug $slug --start=$start --restart=$restart --stop=$stop --browser=$browser"; \
    if [ -n "$report" ]; then args="$args --report $report"; fi; \
    if [ -n "$remote" ]; then args="$args --remote-ssh $remote"; fi; \
    if [ -n "$dns_server" ]; then args="$args --dns-server $dns_server"; fi; \
    if [ -n "$expect_ip" ]; then args="$args --expect-ip $expect_ip"; fi; \
    if [ "$require_remote" = "true" ]; then args="$args --require-remote-tailnet"; fi; \
    go run ./cmd/agentsctl verify workspaces $args
