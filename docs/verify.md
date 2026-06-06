# Verify

This is the standard Vamos verification entrypoint. `/q-verify` must read this file before choosing project-specific checks. It summarizes the verification layers and points to the detailed guides.

## Verification layers

1. **Code generation / build**

   - `templ generate ./server/services/agentchat` when Agent Chat templ files changed.
   - `just build --no-restart` for compile/generation without restarting a managed workspace.

1. **Unit and package tests**

   - Run focused package tests for touched code.
   - Common Agent Chat / Thoughts regression set:
     ```bash
     go test ./server/services/agentchat ./server/services/markdown
     go test ./server/config ./server/services/workspaces ./server/services/agentchat ./cmd/build-agents/internal/build
     ```

1. **Go Story E2E listing and package tests**

   - Required when touching Go Story tests, selectors, steps, Vamos E2E helpers, Agent Chat, Thoughts workbench, route state, or browser-facing behavior:
     ```bash
     go test ./pkg/e2e/tests -list Test
     go test ./pkg/e2e/vamos ./pkg/e2e/tests -run '^$'
     ```
   - Listing/package tests are static checks only. They do not run a browser or prove app behavior.

1. **Workspace/public-host readiness**

   - Required before browser E2E and before asking a human to test a managed feature URL.
   - Use `docs/workspaces-verification.md`.
   - `just build --no-restart` is not enough for browser or human testing. Restart the managed child (`just build` or manager restart action) and verify the public URL reaches the child app, not workspace recovery.

1. **Browser E2E runs**

   - Required for browser-facing changes before human testing.
   - Browser E2E must run against the same public feature URL the human will test. Pass that exact URL with `--base-url`; do not use a different local server unless the human will also test that server.
   - Public workspace E2E authenticates through `/internal/agent-auth/browser-login` with short-lived tokens from `vamos auth playwright-env`. First create a machine key on the manager DB with `vamos auth create-machine-key --database-path <manager-agents.db> --manager-url <manager-url> --slug <slug> --purpose e2e_playwright --purpose verify`, then run `vamos auth login-machine` and `eval "$(vamos auth playwright-env --slug <slug>)"` before `vamos ctl verify workspaces` or Go Story runs.
   - Recommended sequence: managed restart -> confirm public URL healthy -> browser-enabled `vamos ctl verify workspaces` -> `just e2e --base-url <same-public-url> --story <story>` -> human tests `<same-public-url>`.
   - Use `docs/e2e-story-testing.md` for command details, auth, fixture safety, artifacts, and story selection.
   - For Agent Chat, Thoughts chat, URL-state, route, transcript, or QRSPI-next changes, run relevant `durable-session-chat` scenarios at minimum; add `thoughts-workbench` scenarios when document workbench URL/navigation behavior changed.
   - For QRSPI runtime continuation changes, run the single live Pi/Temporal continuation story with `VAMOS_E2E_QRSPI_PROMPT_OVERRIDE=1 just e2e --base-url <feature-url> --story agentchat-qrspi-question-completion-auto-starts-research`; keep cheaper card/reload/sidebar coverage in fixture stories.

1. **Manual human testing**

   - `/q-verify` must ask the user to test the running workspace after automated checks pass and before marking verification complete.
   - Before asking, ensure the feature-slug server for the implementation checkout is actually running and serving the committed code. If needed, run plain `just build` from the feature checkout (not `just build --no-restart`) or the manager restart action, then verify `.vamos/run/status.json` is running and `.vamos/run/workspace.env` contains the feature `VAMOS_WORKSPACE_SLUG`.
   - Include the exact feature URL (`https://<feature-slug>.<workspace-domain>/`) and concise flows to inspect. Do not ask the human to test `main` or a stopped/recovery workspace.

## Required `/q-verify` behavior

- Read this file first as the project verification guide.
- Then read linked detailed guides relevant to the touched surface:
  - `docs/e2e-story-testing.md`
  - `docs/workspaces-verification.md`
- Record commands, artifacts, failures, and skipped checks in `verify.md`.
- If browser E2E or managed restart cannot run, record `blocked` instead of treating static checks as sufficient.
- Do not request human testing until verify-stage fixes are committed and the feature-slug server for this implementation checkout is confirmed running and serving the committed code. For managed workspaces, plain `just build` from the feature checkout is the normal way to build and restart the child server; `just build --no-restart` is compile-only and is not enough for human verification.
