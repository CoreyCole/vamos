# Port Inventory: Durable Chat + E2E to Vamos

## Source
/home/ruby/cn/chestnut-flake/cn-agents-2026-05-19_16-21-15_vamos-durable-session-chat-architecture
vamos-e2e-story-playwright-go_review-fixes
84f6cb65815911b4599cdb03e7420e204822ded9
 M pkg/agents/docs/features/durable-session-chat.story.md
 M pkg/agents/pkg/e2e/generated/durable_session_chat_e2e_test.go
 M pkg/agents/pkg/e2e/steps/catalog.go
 M pkg/agents/pkg/e2e/steps/chat_steps.go
 M pkg/agents/pkg/e2e/story/parse.go
 M pkg/agents/server/services/agentchat/document_workspace.go
 M pkg/agents/server/services/agentchat/embedded_chat.go
 M pkg/agents/server/services/agentchat/session_import.go
 M pkg/agents/server/services/agentchat/workflows/state_store.go
 M pkg/agents/server/services/agentchat/workspace_models.go
 M thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/AGENTS.md
 M thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/verify.md
?? .vamos/
?? thoughts/ccoreycole@gmail.com/
?? thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/handoffs/2026-05-23_14-04-52_verify-handoff.md
?? thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/handoffs/2026-05-23_15-00-48_verify-handoff.md
?? thoughts/creative-mode-agent/plans/2026-05-20_23-02-59_vamos-e2e-story-playwright-go/questions/2026-05-23_14-34-44_copied-workspace-temporal-restart-follow-up.md

## Target
/home/ruby/cn/chestnut-flake/vamos-2026-05-23_14-34-08_port-durable-chat-e2e-to-vamos
main
24fcb6fa48088fe541e72668929627efef0c44cf

## Old E2E/CLI files
cmd/vamos-launcher/bootstrap.go
cmd/vamos-launcher/main.go
cmd/vamos-runtime/internal/ctlcmd/root.go
cmd/vamos-runtime/internal/e2ecmd/check.go
cmd/vamos-runtime/internal/e2ecmd/check_test.go
cmd/vamos-runtime/internal/e2ecmd/fix.go
cmd/vamos-runtime/internal/e2ecmd/fix_test.go
cmd/vamos-runtime/internal/e2ecmd/generate.go
cmd/vamos-runtime/internal/e2ecmd/goldens.go
cmd/vamos-runtime/internal/e2ecmd/review.go
cmd/vamos-runtime/internal/e2ecmd/review_goldens_test.go
cmd/vamos-runtime/internal/e2ecmd/root.go
cmd/vamos-runtime/internal/e2ecmd/root_test.go
cmd/vamos-runtime/internal/e2ecmd/run.go
cmd/vamos-runtime/internal/e2ecmd/run_test.go
cmd/vamos-runtime/internal/rootcmd/root.go
cmd/vamos-runtime/internal/rootcmd/root_test.go
cmd/vamos-runtime/main.go
docs/features/durable-session-chat.story.md
docs/features/thoughts-workbench.story.md
pkg/ctl/verifycmd/http.go
pkg/ctl/verifycmd/playwright.go
pkg/ctl/verifycmd/remote.go
pkg/ctl/verifycmd/report.go
pkg/ctl/verifycmd/workspaces.go
pkg/ctl/verifycmd/workspaces_test.go
pkg/ctl/workspacecmd/config.go
pkg/ctl/workspacecmd/doctor.go
pkg/ctl/workspacecmd/logs.go
pkg/ctl/workspacecmd/main.go
pkg/ctl/workspacecmd/register.go
pkg/ctl/workspacecmd/restart.go
pkg/ctl/workspacecmd/status.go
pkg/ctl/workspacecmd/workspacecmd_test.go
pkg/e2e/artifacts/manifest.go
pkg/e2e/artifacts/plan_bundle.go
pkg/e2e/artifacts/plan_bundle_test.go
pkg/e2e/artifacts/report.go
pkg/e2e/artifacts/report_test.go
pkg/e2e/artifacts/runs/20260521T202252Z/failures.json
pkg/e2e/artifacts/runs/20260521T202252Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202252Z/report.md
pkg/e2e/artifacts/runs/20260521T202318Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202318Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202318Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202318Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202318Z/failures.json
pkg/e2e/artifacts/runs/20260521T202318Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202318Z/report.md
pkg/e2e/artifacts/runs/20260521T202331Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202331Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202331Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202331Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202331Z/failures.json
pkg/e2e/artifacts/runs/20260521T202331Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202331Z/report.md
pkg/e2e/artifacts/runs/20260521T202536Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202536Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202536Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202536Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202536Z/failures.json
pkg/e2e/artifacts/runs/20260521T202536Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202536Z/report.md
pkg/e2e/artifacts/runs/20260521T202656Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202656Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202656Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202656Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202656Z/failures.json
pkg/e2e/artifacts/runs/20260521T202656Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202656Z/report.md
pkg/e2e/artifacts/runs/20260521T202702Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202702Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202702Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202702Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202702Z/failures.json
pkg/e2e/artifacts/runs/20260521T202702Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202702Z/report.md
pkg/e2e/artifacts/runs/20260521T202812Z/failures.json
pkg/e2e/artifacts/runs/20260521T202812Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202812Z/report.md
pkg/e2e/artifacts/runs/20260521T202825Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202825Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202825Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202825Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202825Z/failures.json
pkg/e2e/artifacts/runs/20260521T202825Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202825Z/report.md
pkg/e2e/artifacts/runs/20260521T202854Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202854Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202854Z/failures.json
pkg/e2e/artifacts/runs/20260521T202854Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202854Z/report.md
pkg/e2e/artifacts/runs/20260521T202901Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202901Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202901Z/failures.json
pkg/e2e/artifacts/runs/20260521T202901Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202901Z/report.md
pkg/e2e/artifacts/runs/20260521T202959Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T202959Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T202959Z/failures.json
pkg/e2e/artifacts/runs/20260521T202959Z/manifest.json
pkg/e2e/artifacts/runs/20260521T202959Z/report.md
pkg/e2e/artifacts/runs/20260521T203008Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T203008Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T203008Z/failures.json
pkg/e2e/artifacts/runs/20260521T203008Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203008Z/report.md
pkg/e2e/artifacts/runs/20260521T203026Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T203026Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T203026Z/failures.json
pkg/e2e/artifacts/runs/20260521T203026Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203026Z/report.md
pkg/e2e/artifacts/runs/20260521T203053Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T203053Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T203053Z/failures.json
pkg/e2e/artifacts/runs/20260521T203053Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203053Z/report.md
pkg/e2e/artifacts/runs/20260521T203214Z/failures.json
pkg/e2e/artifacts/runs/20260521T203214Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203214Z/report.md
pkg/e2e/artifacts/runs/20260521T203231Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T203231Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T203231Z/failures.json
pkg/e2e/artifacts/runs/20260521T203231Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203231Z/report.md
pkg/e2e/artifacts/runs/20260521T203353Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T203353Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T203353Z/failures.json
pkg/e2e/artifacts/runs/20260521T203353Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203353Z/report.md
pkg/e2e/artifacts/runs/20260521T203444Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T203444Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T203444Z/failures.json
pkg/e2e/artifacts/runs/20260521T203444Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203444Z/report.md
pkg/e2e/artifacts/runs/20260521T203614Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T203614Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T203614Z/failures.json
pkg/e2e/artifacts/runs/20260521T203614Z/manifest.json
pkg/e2e/artifacts/runs/20260521T203614Z/report.md
pkg/e2e/artifacts/runs/20260521T204556Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T204556Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T204556Z/manifest.json
pkg/e2e/artifacts/runs/20260521T204605Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T204605Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T204605Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T204605Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T204605Z/failures.json
pkg/e2e/artifacts/runs/20260521T204605Z/manifest.json
pkg/e2e/artifacts/runs/20260521T204605Z/report.md
pkg/e2e/artifacts/runs/20260521T204707Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T204707Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T204707Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T204707Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T204707Z/failures.json
pkg/e2e/artifacts/runs/20260521T204707Z/manifest.json
pkg/e2e/artifacts/runs/20260521T204707Z/report.md
pkg/e2e/artifacts/runs/20260521T204741Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T204741Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T204741Z/failures.json
pkg/e2e/artifacts/runs/20260521T204741Z/manifest.json
pkg/e2e/artifacts/runs/20260521T204741Z/report.md
pkg/e2e/artifacts/runs/20260521T205011Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T205011Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T205011Z/failures.json
pkg/e2e/artifacts/runs/20260521T205011Z/manifest.json
pkg/e2e/artifacts/runs/20260521T205011Z/report.md
pkg/e2e/artifacts/runs/20260521T205035Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T205035Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T205035Z/failures.json
pkg/e2e/artifacts/runs/20260521T205035Z/manifest.json
pkg/e2e/artifacts/runs/20260521T205035Z/report.md
pkg/e2e/artifacts/runs/20260521T205058Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T205058Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T205058Z/failures.json
pkg/e2e/artifacts/runs/20260521T205058Z/manifest.json
pkg/e2e/artifacts/runs/20260521T205058Z/report.md
pkg/e2e/artifacts/runs/20260521T205347Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T205347Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T205347Z/failures.json
pkg/e2e/artifacts/runs/20260521T205347Z/manifest.json
pkg/e2e/artifacts/runs/20260521T205347Z/report.md
pkg/e2e/artifacts/runs/20260521T205805Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260521T205805Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260521T205805Z/manifest.json
pkg/e2e/artifacts/runs/20260523T180318Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T180318Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T180318Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T180318Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T180318Z/failures.json
pkg/e2e/artifacts/runs/20260523T180318Z/manifest.json
pkg/e2e/artifacts/runs/20260523T180318Z/report.md
pkg/e2e/artifacts/runs/20260523T180853Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T180853Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T180853Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T180853Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T180853Z/failures.json
pkg/e2e/artifacts/runs/20260523T180853Z/manifest.json
pkg/e2e/artifacts/runs/20260523T180853Z/report.md
pkg/e2e/artifacts/runs/20260523T181129Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T181129Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T181129Z/failures.json
pkg/e2e/artifacts/runs/20260523T181129Z/manifest.json
pkg/e2e/artifacts/runs/20260523T181129Z/report.md
pkg/e2e/artifacts/runs/20260523T181232Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T181232Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T181232Z/manifest.json
pkg/e2e/artifacts/runs/20260523T181252Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T181252Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T181252Z/failures.json
pkg/e2e/artifacts/runs/20260523T181252Z/manifest.json
pkg/e2e/artifacts/runs/20260523T181252Z/report.md
pkg/e2e/artifacts/runs/20260523T181907Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T181907Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T181907Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T181907Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T181907Z/failures.json
pkg/e2e/artifacts/runs/20260523T181907Z/manifest.json
pkg/e2e/artifacts/runs/20260523T181907Z/report.md
pkg/e2e/artifacts/runs/20260523T182120Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T182120Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T182120Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T182120Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T182120Z/manifest.json
pkg/e2e/artifacts/runs/20260523T200037Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T200037Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T200037Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T200037Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T200037Z/failures.json
pkg/e2e/artifacts/runs/20260523T200037Z/manifest.json
pkg/e2e/artifacts/runs/20260523T200037Z/report.md
pkg/e2e/artifacts/runs/20260523T200341Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T200341Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T200341Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T200341Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T200341Z/failures.json
pkg/e2e/artifacts/runs/20260523T200341Z/manifest.json
pkg/e2e/artifacts/runs/20260523T200341Z/report.md
pkg/e2e/artifacts/runs/20260523T200659Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T200659Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T200659Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T200659Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T200659Z/failures.json
pkg/e2e/artifacts/runs/20260523T200659Z/manifest.json
pkg/e2e/artifacts/runs/20260523T200659Z/report.md
pkg/e2e/artifacts/runs/20260523T201639Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T201639Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T201639Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T201639Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T201639Z/failures.json
pkg/e2e/artifacts/runs/20260523T201639Z/manifest.json
pkg/e2e/artifacts/runs/20260523T201639Z/report.md
pkg/e2e/artifacts/runs/20260523T202208Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T202208Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T202208Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T202208Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T202208Z/failures.json
pkg/e2e/artifacts/runs/20260523T202208Z/manifest.json
pkg/e2e/artifacts/runs/20260523T202208Z/report.md
pkg/e2e/artifacts/runs/20260523T202814Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T202814Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T202814Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T202814Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T202814Z/manifest.json
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211041Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211041Z/failures.json
pkg/e2e/artifacts/runs/20260523T211041Z/manifest.json
pkg/e2e/artifacts/runs/20260523T211041Z/report.md
pkg/e2e/artifacts/runs/20260523T211528Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211528Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211528Z/failures.json
pkg/e2e/artifacts/runs/20260523T211528Z/manifest.json
pkg/e2e/artifacts/runs/20260523T211528Z/report.md
pkg/e2e/artifacts/runs/20260523T211642Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211642Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211642Z/failures.json
pkg/e2e/artifacts/runs/20260523T211642Z/manifest.json
pkg/e2e/artifacts/runs/20260523T211642Z/report.md
pkg/e2e/artifacts/runs/20260523T211752Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211752Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211752Z/failures.json
pkg/e2e/artifacts/runs/20260523T211752Z/manifest.json
pkg/e2e/artifacts/runs/20260523T211752Z/report.md
pkg/e2e/artifacts/runs/20260523T211857Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T211857Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T211857Z/failures.json
pkg/e2e/artifacts/runs/20260523T211857Z/manifest.json
pkg/e2e/artifacts/runs/20260523T211857Z/report.md
pkg/e2e/artifacts/runs/20260523T212423Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T212423Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T212423Z/failures.json
pkg/e2e/artifacts/runs/20260523T212423Z/manifest.json
pkg/e2e/artifacts/runs/20260523T212423Z/report.md
pkg/e2e/artifacts/runs/20260523T212518Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T212518Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T212518Z/failures.json
pkg/e2e/artifacts/runs/20260523T212518Z/manifest.json
pkg/e2e/artifacts/runs/20260523T212518Z/report.md
pkg/e2e/artifacts/runs/20260523T212714Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T212714Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T212714Z/failures.json
pkg/e2e/artifacts/runs/20260523T212714Z/manifest.json
pkg/e2e/artifacts/runs/20260523T212714Z/report.md
pkg/e2e/artifacts/runs/20260523T212827Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T212827Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T212827Z/failures.json
pkg/e2e/artifacts/runs/20260523T212827Z/manifest.json
pkg/e2e/artifacts/runs/20260523T212827Z/report.md
pkg/e2e/artifacts/runs/20260523T212918Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T212918Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T212918Z/manifest.json
pkg/e2e/artifacts/runs/20260523T212935Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T212935Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T212935Z/failures.json
pkg/e2e/artifacts/runs/20260523T212935Z/manifest.json
pkg/e2e/artifacts/runs/20260523T212935Z/report.md
pkg/e2e/artifacts/runs/20260523T213019Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213019Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213019Z/manifest.json
pkg/e2e/artifacts/runs/20260523T213109Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213109Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213109Z/manifest.json
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213143Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213143Z/failures.json
pkg/e2e/artifacts/runs/20260523T213143Z/manifest.json
pkg/e2e/artifacts/runs/20260523T213143Z/report.md
pkg/e2e/artifacts/runs/20260523T213400Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213400Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213400Z/failures.json
pkg/e2e/artifacts/runs/20260523T213400Z/manifest.json
pkg/e2e/artifacts/runs/20260523T213400Z/report.md
pkg/e2e/artifacts/runs/20260523T213838Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213838Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213838Z/failures.json
pkg/e2e/artifacts/runs/20260523T213838Z/manifest.json
pkg/e2e/artifacts/runs/20260523T213838Z/report.md
pkg/e2e/artifacts/runs/20260523T213923Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T213923Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T213923Z/manifest.json
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/freeform-chat-fixture-replays-durable-transcript/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/freeform-chat-started-from-thoughts-root-survives-refresh-and-resume/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/qrspi-plan-workspace-chat-updates-verification-artifact-through-pi-and-temporal/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.html
pkg/e2e/artifacts/runs/20260523T214011Z/durable-session-chat/workspace-switching-restores-each-workspace-latest-chat/desktop-full/page.png
pkg/e2e/artifacts/runs/20260523T214011Z/manifest.json
pkg/e2e/artifacts/screenshots.go
pkg/e2e/fixtures/chat_sessions.go
pkg/e2e/fixtures/registry.go
pkg/e2e/fixtures/registry_test.go
pkg/e2e/fixtures/thoughts.go
pkg/e2e/generate/check.go
pkg/e2e/generated/durable_session_chat_e2e_test.go
pkg/e2e/generated/thoughts_workbench_e2e_test.go
pkg/e2e/generate/generate.go
pkg/e2e/generate/generate_test.go
pkg/e2e/generate/write.go
pkg/e2e/goldens/goldens.go
pkg/e2e/goldens/goldens_test.go
pkg/e2e/repair/repair.go
pkg/e2e/repair/repair_test.go
pkg/e2e/repair/validate.go
pkg/e2e/review/markdown.go
pkg/e2e/review/markdown_test.go
pkg/e2e/review/review.go
pkg/e2e/runtime/artifacts.go
pkg/e2e/runtime/auth.go
pkg/e2e/runtime/config.go
pkg/e2e/runtime/config_test.go
pkg/e2e/runtime/scenario.go
pkg/e2e/runtime/timeout.go
pkg/e2e/runtime/viewports.go
pkg/e2e/runtime/viewports_test.go
pkg/e2e/runtime/workspace.go
pkg/e2e/runtime/workspace_test.go
pkg/e2e/selectors/catalog.go
pkg/e2e/selectors/catalog_test.go
pkg/e2e/steps/catalog.go
pkg/e2e/steps/catalog_test.go
pkg/e2e/steps/chat_steps.go
pkg/e2e/steps/filesystem_steps.go
pkg/e2e/steps/filesystem_steps_test.go
pkg/e2e/steps/fixture_steps_test.go
pkg/e2e/steps/noop_steps.go
pkg/e2e/story/parse.go
pkg/e2e/story/parse_test.go
pkg/e2e/story/properties.go
pkg/e2e/story/properties_test.go
pkg/e2e/story/types.go
pkg/e2e/story/validate.go
pkg/e2e/story/validate_test.go

## Target pre-existing matching paths

## Durable chat semantic-merge candidates
### server/services/agentchat/chat_session_handlers.go
8eac7ef9 feat(agentchat): add chat annotations for workspace slice 6
556a2abc feat(chatsession): add projection replay and SSE endpoints for workspace slice 3
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### server/services/agentchat/chat_session_integration.go
580597f9 feat(chatsession): add fork baselines and active path for workspace slice 5
75c9b7f9 feat(agentchat): write run activity into chat sessions for workspace slice 4
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### server/services/agentchat/document_workspace.go
c1bdf601 refactor(agentchat): delete artifact-era doc paths for review plan slice 7
848939a3 feat(docs): add doc workspace schema for review plan slice 1
1de33028 feat(agent-chat): start chats from any document
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### server/services/agentchat/embedded_chat.go
40e94515 fix(agentchat): restore embedded chat selection for review plan slice 2
8835a739 fix: restore unified thoughts chat workbench
741aeedd fix(agentchat): preserve doc context in embedded stream URLs
50c76331 fix(agentchat): normalize embedded doc attachment paths for workspace slice 6
92d7ded4 feat(agentchat): render always-open embedded chat panel for workspace slice 2
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### server/services/agentchat/session_import.go
848939a3 feat(docs): add doc workspace schema for review plan slice 1
791c9469 agent-chat-freeform-cleanup-review slice 4: share comments by canonical document path
0b35850f agent-chat-freeform-cleanup-review slice 1: rename pkg agents module to Vamos
8ce67986 Relax pi session reuse ownership checks
883feaf8 Add global pi session workspace resolution
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### server/services/agentchat/workflows/state_store.go
848939a3 feat(docs): add doc workspace schema for review plan slice 1
7f947e37 feat(agentchat): persist QRSPI implementation cwd for workspace slice 3
0fe9037c feat(qrspi): add conditional graph path for workspace review loops slice 1
791c9469 agent-chat-freeform-cleanup-review slice 4: share comments by canonical document path
0b35850f agent-chat-freeform-cleanup-review slice 1: rename pkg agents module to Vamos
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### server/services/agentchat/workspace_models.go
848939a3 feat(docs): add doc workspace schema for review plan slice 1
791c9469 agent-chat-freeform-cleanup-review slice 4: share comments by canonical document path
0b35850f agent-chat-freeform-cleanup-review slice 1: rename pkg agents module to Vamos
ee32abc3 extract nested pkg agents module
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### pkg/db/migrations/schema.sql
8f7572db feat(chatsession): add durable chat session schema for workspace slice 1
b69dd89a feat(agentchat): add embedded chat URL state persistence for workspace slice 1
10c47867 feat(comments): reset comments around document paths for layout correction slice 6
848939a3 feat(docs): add doc workspace schema for review plan slice 1
ed9768d0 feat(workspaces): add shared workspace slug schema for follow-up slice 1
7e182d8 feat: initialize extracted Vamos server for workspace slice 5
### pkg/db/queries/impl_workspaces.sql
7e182d8 feat: initialize extracted Vamos server for workspace slice 5

## Conflict Ledger

| Area | Old path | New path | Decision | Rationale |
|---|---|---|---|---|
| E2E CLI | cmd/vamos-runtime | cmd/vamos-runtime | copy_new | absent in target |
| E2E packages | pkg/e2e | pkg/e2e | copy_new_then_lint | absent in target; generated tests regenerated later |
| ctl | pkg/ctl | pkg/ctl | copy_new | absent in target; `cmd/agentsctl` can reuse later if needed |
| durable schema | pkg/db/*impl_workspaces* | pkg/db/* | semantic_merge | old deletes `impl_workspaces`; target still has it; ADR says old stack authoritative |
| embedded freeform | server/services/agentchat/embedded_chat.go | same | semantic_merge | old unstaged fix required by refresh/resume story |
