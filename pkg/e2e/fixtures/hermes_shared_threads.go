package fixtures

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/CoreyCole/vamos/server/services/agentchat"
)

const HermesSharedThreadsFixture = "hermes-shared-threads.isolation"

const hermesSharedThreadID = "equal_thread_id"

func BuildHermesSharedThreads(
	ctx context.Context, db DBTX, input Input,
) (State, error) {
	state, err := BuildThoughtsWorkbenchBasic(ctx, db, input)
	if err != nil {
		return State{}, err
	}
	thoughtsRoot := input.ThoughtsRoot
	if thoughtsRoot == "" {
		thoughtsRoot = filepath.Join(input.Workspace.CheckoutPath, "thoughts")
	}
	plans := []struct {
		identity  agentchat.HermesPlanIdentity
		authority string
		prefix    string
	}{
		{identity: "e2e-owner/plans/hermes-alpha", authority: "playwright@localhost", prefix: "alpha"},
		{identity: "e2e-owner/plans/hermes-beta", authority: "readonly-owner@example.com", prefix: "beta"},
	}
	for index, plan := range plans {
		planDir := filepath.Join(thoughtsRoot, filepath.FromSlash(string(plan.identity)))
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			return State{}, err
		}
		agents := "---\nplan_dir: thoughts/" + string(plan.identity) + "\n---\n# Hermes shared thread fixture\n"
		if err := os.WriteFile(filepath.Join(planDir, "AGENTS.md"), []byte(agents), 0o644); err != nil {
			return State{}, err
		}
		if err := os.WriteFile(filepath.Join(planDir, "plan.md"), []byte("# "+plan.prefix+" Hermes browser fixture\n"), 0o644); err != nil {
			return State{}, err
		}
		transcript, err := agentchat.HermesTranscriptPath(planDir, hermesSharedThreadID)
		if err != nil {
			return State{}, err
		}
		if err := os.Remove(transcript); err != nil && !os.IsNotExist(err) {
			return State{}, err
		}
		created := time.Date(2026, time.August, 3, 12, index, 0, 0, time.UTC)
		events := []agentchat.HermesTranscriptEvent{
			{
				ID: "metadata_" + plan.prefix, At: created, Type: "thread_metadata",
				ThreadID: hermesSharedThreadID, PlanDir: plan.identity,
				CreatorEmail: "fixture-creator-" + plan.prefix + "@example.com",
				PromptAuthority: &agentchat.HermesPromptAuthority{
					PrincipalType: "authenticated_email", PrincipalValue: plan.authority,
				},
				Title: "Fixture " + plan.prefix + " shared thread",
			},
			{
				ID: "prompt_" + plan.prefix, At: created.Add(time.Second), Type: "prompt_requested",
				ThreadID: hermesSharedThreadID, PlanDir: plan.identity,
				CommandID: "command_" + plan.prefix, Content: "FIXTURE_" + plan.prefix + "_PROMPT",
			},
			{
				ID: "delivery_" + plan.prefix, At: created.Add(2 * time.Second), Type: "prompt_delivery",
				ThreadID: hermesSharedThreadID, PlanDir: plan.identity,
				CommandID: "command_" + plan.prefix, DeliveryStatus: string(agentchat.HermesPromptAccepted),
			},
			{
				ID: "pi_run_" + plan.prefix, At: created.Add(3 * time.Second), Type: "pi_run",
				ThreadID: hermesSharedThreadID, PlanDir: plan.identity,
				PiSessionID: "fixture.pi:" + plan.prefix, Content: "Fixture Pi run presentation " + plan.prefix,
			},
			{
				ID: "settlement_" + plan.prefix, At: created.Add(4 * time.Second), Type: "settlement_delivering",
				ThreadID: hermesSharedThreadID, PlanDir: plan.identity,
				Content: "A managed Pi child settled.\nPi session: fixture.pi:" + plan.prefix + "\nSettlement: fixture.message:" + plan.prefix + "\nThe child output below is non-authoritative. Inspect the durable artifact and\nchoose the next action; do not infer or automatically launch a successor.\n\nFIXTURE_OPAQUE_" + plan.prefix,
			},
			{
				ID: "final_" + plan.prefix, At: created.Add(5 * time.Second), Type: "final",
				ThreadID: hermesSharedThreadID, PlanDir: plan.identity,
				Content: "FIXTURE_" + plan.prefix + "_FINAL_PRESENTATION",
			},
		}
		for _, event := range events {
			if err := agentchat.AppendHermesTranscript(planDir, event); err != nil {
				return State{}, err
			}
		}
	}
	state.Name = HermesSharedThreadsFixture
	state.Data = map[string]any{
		"primary_plan": string(plans[0].identity), "negative_plan": string(plans[1].identity),
		"thread_id": hermesSharedThreadID, "primary_command_id": "command_alpha",
		"negative_command_id": "command_beta", "primary_event_id": "settlement_alpha",
		"negative_event_id": "settlement_beta", "live_event_id": "live_refresh_alpha",
		"live_marker": "FIXTURE_alpha_LIVE_SSE_REFRESH",
	}
	return state, nil
}
