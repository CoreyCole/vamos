package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/server/services/markdown"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

const probeUserEmail = "workspace-probe@vamos.local"

func probeTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return 20 * time.Second
}

func (s *Service) RunWorkspaceProbe(
	ctx context.Context,
	req workspaces.AgentChatProbeRequest,
) (workspaces.AgentChatProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout(req.Timeout))
	defer cancel()

	workspaceRecord, err := s.GetOrCreateWorkspaceForRootDocPath(ctx, markdown.ChatWorkspaceOpenInput{
		UserEmail:    probeUserEmail,
		RootDocPath:  s.thoughtsRoot,
		Title:        "Workspace isolation probe",
		WorkflowType: string(WorkspaceWorkflowFreeform),
		Source:       string(WorkspaceSourceTerminal),
	})
	if err != nil {
		return workspaces.AgentChatProbeResult{Error: err.Error()}, err
	}
	thread, run, _, err := s.StartWorkspaceThread(
		ctx,
		workspaceRecord.ID,
		probeUserEmail,
		"Workspace isolation verification probe. Do not edit files.",
	)
	if err != nil {
		return workspaces.AgentChatProbeResult{Error: err.Error()}, err
	}
	latestRun, err := s.queries.GetAgentRun(ctx, run.ID)
	if err != nil {
		return workspaces.AgentChatProbeResult{RunID: run.ID, Error: err.Error()}, err
	}
	prepared, err := s.buildRunInput(ctx, *thread, latestRun)
	if err != nil {
		return workspaces.AgentChatProbeResult{RunID: latestRun.ID, Error: err.Error()}, err
	}
	result := workspaces.AgentChatProbeResult{
		RunID:                  latestRun.ID,
		WorkflowID:             latestRun.WorkflowID,
		CallbackEndpoint:       prepared.Input.CallbackEndpoint,
		SnapshotLoaderEndpoint: prepared.Input.SnapshotLoaderEndpoint,
		Cwd:                    prepared.Input.Cwd,
		TemporalAddress:        strings.TrimSpace(os.Getenv("TEMPORAL_ADDRESS")),
	}
	if err := s.probeSnapshotEndpoint(ctx, prepared.Input); err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.ReachedSnapshotLoader = true
	if err := s.probeCallbackEndpoint(ctx, prepared.Input); err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.ReachedCallback = true
	return result, nil
}

func (s *Service) probeSnapshotEndpoint(ctx context.Context, input conversation.RunInput) error {
	endpoint := input.SnapshotLoaderEndpoint + "?run_id=" + url.QueryEscape(input.RunID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(os.Getenv("VAMOS_INTERNAL_TOKEN")); token != "" {
		req.Header.Set("X-Vamos-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("snapshot probe HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *Service) probeCallbackEndpoint(ctx context.Context, input conversation.RunInput) error {
	failure, err := json.Marshal(conversation.RunFailure{
		WorkspaceID:   input.WorkspaceID,
		SessionID:     input.SessionID,
		ChatSessionID: input.ChatSessionID,
		RunID:         input.RunID,
		ThreadID:      input.ThreadID,
		RootDocPath:   input.RootDocPath,
		ErrorMessage:  "workspace isolation probe complete",
		EventKey:      input.RunID + ":probe",
	})
	if err != nil {
		return err
	}
	payload, err := json.Marshal(conversation.EventEnvelope{
		WorkspaceID:   input.WorkspaceID,
		SessionID:     input.SessionID,
		ChatSessionID: input.ChatSessionID,
		RunID:         input.RunID,
		ThreadID:      input.ThreadID,
		EventType:     conversation.EventRunFailed,
		EventKey:      input.RunID + ":probe",
		PayloadJSON:   string(failure),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, input.CallbackEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(os.Getenv("VAMOS_INTERNAL_TOKEN")); token != "" {
		req.Header.Set("X-Vamos-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("callback probe HTTP %d", resp.StatusCode)
	}
	return nil
}
