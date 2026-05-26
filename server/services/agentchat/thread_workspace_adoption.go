package agentchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
	"github.com/CoreyCole/vamos/server/services/markdown"
)

type ThreadWorkspaceAdoptionInput struct {
	ThreadID string
	RunID    string
	Source   string
}

type ThreadWorkspaceAdoptionResult struct {
	PrimaryWorkspaceID    string
	RelatedWorkspaceIDs   []string
	AppliedWorkflowResult bool
	Adopted               bool
}

func (s *Service) AdoptThreadWorkspacesForRun(ctx context.Context, input ThreadWorkspaceAdoptionInput) (ThreadWorkspaceAdoptionResult, error) {
	run, err := s.queries.GetAgentRun(ctx, strings.TrimSpace(input.RunID))
	if err != nil {
		return ThreadWorkspaceAdoptionResult{}, err
	}
	threadID := strings.TrimSpace(input.ThreadID)
	if threadID == "" {
		threadID = run.ThreadID
	}
	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return ThreadWorkspaceAdoptionResult{}, err
	}
	entries, err := s.queries.ListAgentEntriesByRun(ctx, nullString(run.ID))
	if err != nil {
		return ThreadWorkspaceAdoptionResult{}, err
	}
	writeRoots := s.resolveAttachableRootsFromEntries(entries, thread.Cwd)
	headEntryID := resultHeadOrThreadHead(run, thread)
	assistantText := finalAssistantTextFromRunEntries(entries)
	if strings.TrimSpace(assistantText) == "" && headEntryID != "" {
		assistantText, _ = s.finalAssistantTextForAdoption(ctx, thread.ID, headEntryID)
	}
	xmlRoots := s.resolveAttachableRootsFromQRSPIXML(ctx, assistantText, thread.Cwd)
	primary, related := choosePrimaryAndRelatedRoots(xmlRoots, writeRoots)
	if primary.RelPath == "" {
		return ThreadWorkspaceAdoptionResult{}, nil
	}
	primaryWorkspace, err := s.EnsureQRSPIWorkspaceForRoot(ctx, thread.UserEmail, primary)
	if err != nil {
		return ThreadWorkspaceAdoptionResult{}, err
	}
	if err := s.SetThreadPrimaryWorkspace(ctx, thread.ID, primaryWorkspace.ID, input.Source); err != nil {
		return ThreadWorkspaceAdoptionResult{}, err
	}
	out := ThreadWorkspaceAdoptionResult{PrimaryWorkspaceID: primaryWorkspace.ID, Adopted: true}
	for _, root := range related {
		workspace, err := s.EnsureQRSPIWorkspaceForRoot(ctx, thread.UserEmail, root)
		if err != nil {
			return out, err
		}
		if workspace.ID == primaryWorkspace.ID {
			continue
		}
		if err := s.AddThreadRelatedWorkspace(ctx, thread.ID, workspace.ID, input.Source); err != nil {
			return out, err
		}
		out.RelatedWorkspaceIDs = append(out.RelatedWorkspaceIDs, workspace.ID)
	}
	if strings.Contains(assistantText, "<qrspi-result>") && headEntryID != "" && !run.WorkflowNodeID.Valid {
		applied, err := s.ApplyQRSPIResultToAdoptedWorkspace(ctx, primaryWorkspace.ID, thread.ID, headEntryID)
		if err != nil {
			return out, err
		}
		out.AppliedWorkflowResult = applied
	}
	return out, nil
}

func (s *Service) ResolveAttachableRootsFromRun(ctx context.Context, run db.AgentRun) ([]AttachablePlanRoot, error) {
	thread, err := s.queries.GetAgentThread(ctx, run.ThreadID)
	if err != nil {
		return nil, err
	}
	entries, err := s.queries.ListAgentEntriesByRun(ctx, nullString(run.ID))
	if err != nil {
		return nil, err
	}
	return s.resolveAttachableRootsFromEntries(entries, thread.Cwd), nil
}

func (s *Service) resolveAttachableRootsFromEntries(entries []db.AgentEntry, cwd string) []AttachablePlanRoot {
	toolCalls := map[string]toolCallRef{}
	seen := map[string]struct{}{}
	roots := []AttachablePlanRoot{}
	for _, row := range entries {
		entry, ok := agentEntryToPiSessionEntry(row)
		if !ok {
			continue
		}
		before := map[string]struct{}{}
		collectTouchedPlanDirsFromEntry(entry, s.thoughtsRoot, cwd, toolCalls, before)
		if strings.TrimSpace(entry.Message.Role) != "toolResult" || entry.Message.IsError ||
			(strings.TrimSpace(entry.Message.ToolName) != "write" && strings.TrimSpace(entry.Message.ToolName) != "edit") {
			continue
		}
		for _, rawPath := range extractPathStrings(entry.Message.Details, entry.Message.Content) {
			if root, ok := s.ResolveAttachablePlanRootFrom(rawPath, cwd); ok {
				before[root.AbsPath] = struct{}{}
			}
		}
		if call, ok := toolCalls[strings.TrimSpace(entry.Message.ToolCallID)]; ok {
			for _, rawPath := range extractPathStrings(call.Arguments) {
				if root, ok := s.ResolveAttachablePlanRootFrom(rawPath, cwd); ok {
					before[root.AbsPath] = struct{}{}
				}
			}
		}
		for path := range before {
			root, ok := s.ResolveAttachablePlanRootFrom(path, cwd)
			if !ok {
				continue
			}
			if _, exists := seen[root.RelPath]; exists {
				continue
			}
			seen[root.RelPath] = struct{}{}
			roots = append(roots, root)
			if root.IsNested && root.ParentRelPath != "" {
				parentPath := filepath.Join(s.thoughtsRoot, filepath.FromSlash(root.ParentRelPath))
				if parent, ok := s.ResolveAttachablePlanRootFrom(parentPath, cwd); ok {
					if _, exists := seen[parent.RelPath]; !exists {
						seen[parent.RelPath] = struct{}{}
						roots = append(roots, parent)
					}
				}
			}
		}
	}
	return roots
}

func agentEntryToPiSessionEntry(row db.AgentEntry) (PiSessionEntry, bool) {
	var entry PiSessionEntry
	if err := json.Unmarshal([]byte(row.PayloadJson), &entry); err != nil {
		return PiSessionEntry{}, false
	}
	if entry.ID == "" {
		entry.ID = row.EntryID
	}
	return entry, true
}

func (s *Service) ResolveAttachableRootsFromQRSPIXML(ctx context.Context, text string) []AttachablePlanRoot {
	return s.resolveAttachableRootsFromQRSPIXML(ctx, text, "")
}

func (s *Service) resolveAttachableRootsFromQRSPIXML(ctx context.Context, text string, cwd string) []AttachablePlanRoot {
	_ = ctx
	if !strings.Contains(text, "<qrspi-result>") {
		return nil
	}
	parsedAny, err := qrspi.QRSPIXMLParser{}.Parse(text, wruntime.ParseContext{})
	if err != nil {
		return nil
	}
	parsed, ok := parsedAny.(qrspi.ResultXML)
	if !ok {
		return nil
	}
	candidates := []string{
		parsed.Workspace,
		parsed.WorkspaceMetadata.PlanWorkspace,
		parsed.WorkspaceMetadata.ImplementationWorkspace,
		parsed.Artifact,
	}
	for _, artifact := range parsed.Artifacts {
		candidates = append(candidates, artifact.Path)
	}
	seen := map[string]struct{}{}
	roots := []AttachablePlanRoot{}
	for _, candidate := range candidates {
		root, ok := s.ResolveAttachablePlanRootFrom(candidate, cwd)
		if !ok {
			continue
		}
		if _, exists := seen[root.RelPath]; exists {
			continue
		}
		seen[root.RelPath] = struct{}{}
		roots = append(roots, root)
		if root.IsNested && root.ParentRelPath != "" {
			parentPath := filepath.Join(s.thoughtsRoot, filepath.FromSlash(root.ParentRelPath))
			if parent, ok := s.ResolveAttachablePlanRootFrom(parentPath, cwd); ok {
				if _, exists := seen[parent.RelPath]; !exists {
					seen[parent.RelPath] = struct{}{}
					roots = append(roots, parent)
				}
			}
		}
	}
	return roots
}

func choosePrimaryAndRelatedRoots(xmlRoots, writeRoots []AttachablePlanRoot) (AttachablePlanRoot, []AttachablePlanRoot) {
	ordered := append([]AttachablePlanRoot{}, xmlRoots...)
	ordered = append(ordered, writeRoots...)
	seen := map[string]struct{}{}
	var primary AttachablePlanRoot
	related := []AttachablePlanRoot{}
	for _, root := range ordered {
		if root.RelPath == "" {
			continue
		}
		if _, exists := seen[root.RelPath]; exists {
			continue
		}
		seen[root.RelPath] = struct{}{}
		if primary.RelPath == "" {
			primary = root
			continue
		}
		related = append(related, root)
	}
	return primary, related
}

func (s *Service) EnsureQRSPIWorkspaceForRoot(ctx context.Context, userEmail string, root AttachablePlanRoot) (db.Workspace, error) {
	if strings.TrimSpace(root.AbsPath) == "" {
		return db.Workspace{}, fmt.Errorf("attachable root path is required")
	}
	workspace, err := s.GetOrCreateWorkspaceForRootDocPath(ctx, markdown.ChatWorkspaceOpenInput{
		UserEmail:    userEmail,
		RootDocPath:  root.AbsPath,
		Title:        planWorkspaceLabel(root.AbsPath),
		WorkflowType: string(WorkspaceWorkflowQRSPI),
		Source:       string(WorkspaceSourceWeb),
	})
	if err != nil {
		return db.Workspace{}, err
	}
	if strings.TrimSpace(workspace.WorkflowType) != string(WorkspaceWorkflowQRSPI) {
		if err := s.queries.UpdateWorkspaceWorkflowState(ctx, db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      string(WorkspaceWorkflowQRSPI),
			WorkflowStateJson: workspace.WorkflowStateJson,
		}); err != nil {
			return db.Workspace{}, err
		}
		workspace, err = s.queries.GetWorkspace(ctx, workspace.ID)
		if err != nil {
			return db.Workspace{}, err
		}
	}
	_, _ = s.SyncWorkspaceDocInventory(ctx, workspace)
	return workspace, nil
}

func (s *Service) ApplyQRSPIResultToAdoptedWorkspace(ctx context.Context, workspaceID, threadID, headEntryID string) (bool, error) {
	adapter, ok := s.workflowService.(*agentchatworkflows.Service)
	if !ok || adapter == nil || adapter.Definitions == nil || adapter.Store == nil {
		return false, nil
	}
	def, ok := adapter.Definitions.Get(qrspi.AgentChatWorkflowType)
	if !ok {
		return false, nil
	}
	state, err := adapter.Store.LoadWorkspaceState(ctx, workspaceID)
	if err != nil {
		policy, policyErr := json.Marshal(qrspi.DefaultPolicy())
		if policyErr != nil {
			return false, policyErr
		}
		state, err = wruntime.InitialState(def, policy)
		if err != nil {
			return false, err
		}
	}
	currentDef, ok := adapter.Definitions.Get(wruntime.WorkflowID(strings.TrimSpace(state.Type)))
	if !ok {
		return false, nil
	}
	assistant, err := adapter.Store.FinalAssistantText(ctx, threadID, headEntryID)
	if err != nil {
		return false, err
	}
	parseCtx := wruntime.ParseContext{
		WorkflowType: strings.TrimSpace(state.Type),
		ThreadID:     threadID,
		HeadEntryID:  headEntryID,
	}
	parsed, err := currentDef.ResultParser.Parse(assistant, parseCtx)
	if err != nil {
		return false, nil
	}
	workflowResult, err := currentDef.ResultConverter.ToWorkflowResult(parsed, parseCtx)
	if err != nil {
		return false, err
	}
	_, applied, err := adapter.ApplyExternalWorkflowResult(ctx, agentchatworkflows.ExternalWorkflowResultInput{
		WorkspaceID: workspaceID,
		ThreadID:    threadID,
		HeadEntryID: headEntryID,
		State:       state,
		Result:      workflowResult,
	})
	if err != nil {
		return false, err
	}
	if applied {
		_ = s.queries.UpdateWorkspaceSelectedThread(ctx, db.UpdateWorkspaceSelectedThreadParams{ID: workspaceID, SelectedThreadID: nullString(threadID)})
	}
	return applied, nil
}

func (s *Service) finalAssistantTextForAdoption(ctx context.Context, threadID, headEntryID string) (string, error) {
	adapter, ok := s.workflowService.(*agentchatworkflows.Service)
	if !ok || adapter == nil || adapter.Store == nil {
		return "", errors.New("workflow store is not configured")
	}
	return adapter.Store.FinalAssistantText(ctx, threadID, headEntryID)
}

func finalAssistantTextFromRunEntries(entries []db.AgentEntry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		text := assistantTextFromPayload(entries[i].PayloadJson)
		if strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func assistantTextFromPayload(payload string) string {
	var envelope struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return ""
	}
	if strings.TrimSpace(envelope.Type) != "message" || strings.TrimSpace(envelope.Message.Role) != "assistant" {
		return ""
	}
	return extractContentText(envelope.Message.Content)
}

func resultHeadOrThreadHead(run db.AgentRun, thread db.AgentThread) string {
	if run.ResultHeadEntryID.Valid && strings.TrimSpace(run.ResultHeadEntryID.String) != "" {
		return strings.TrimSpace(run.ResultHeadEntryID.String)
	}
	if thread.HeadEntryID.Valid {
		return strings.TrimSpace(thread.HeadEntryID.String)
	}
	return ""
}
