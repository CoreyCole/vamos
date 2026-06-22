package agentchat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/db"
)

func TestAdvanceQRSPIWorkflowUsesRuntimePendingContinuation(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	thread := mustCreateAgentThread(t, service, "thread-1", "user@example.com", "/repo", "lineage-1")
	if err := service.SetThreadPrimaryWorkspace(context.Background(), thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace() error = %v", err)
	}
	fake := &fakeWorkflowCompletionService{}
	service.workflowService = fake

	next, err := service.AdvanceQRSPIWorkflow(context.Background(), "user@example.com", thread.ID)
	if err != nil {
		t.Fatalf("AdvanceQRSPIWorkflow() error = %v", err)
	}
	if next.ID != thread.ID {
		t.Fatalf("next thread = %q, want source thread %q", next.ID, thread.ID)
	}
	if fake.workspaceID != workspace.ID || fake.userEmail != "user@example.com" {
		t.Fatalf("runtime continue = workspace %q user %q, want %s", fake.workspaceID, fake.userEmail, workspace.ID)
	}
}

type fakeWorkflowCompletionService struct {
	workspaceID string
	userEmail   string
}

func (f *fakeWorkflowCompletionService) OnRunComplete(context.Context, conversation.RunResult) error {
	return nil
}

func (f *fakeWorkflowCompletionService) AdvanceHumanGate(_ context.Context, workspaceID, userEmail string) (string, error) {
	f.workspaceID = workspaceID
	f.userEmail = userEmail
	return "run-1", nil
}

func TestCreateThreadFromWorkspaceTargets(t *testing.T) {
	service := newTestAgentChatService(t)
	primary := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	related := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	thread := mustCreateAgentThread(t, service, "thread-targets", "user@example.com", "/tmp/source", "lineage-targets")
	if err := service.SetThreadPrimaryWorkspace(context.Background(), thread.ID, primary.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace() error = %v", err)
	}
	if err := service.AddThreadRelatedWorkspace(context.Background(), thread.ID, related.ID, "test"); err != nil {
		t.Fatalf("AddThreadRelatedWorkspace() error = %v", err)
	}

	nextRelated, err := service.CreateThreadFromWorkspace(context.Background(), CreateThreadFromWorkspaceInput{
		UserEmail:         "user@example.com",
		SourceThreadID:    thread.ID,
		TargetWorkspaceID: related.ID,
		TargetKind:        NewThreadTargetRelated,
	})
	if err != nil {
		t.Fatalf("CreateThreadFromWorkspace(related) error = %v", err)
	}
	selected, ok, err := service.ResolvePrimaryWorkspaceForThread(context.Background(), "user@example.com", nextRelated.ID)
	if err != nil || !ok || selected.ID != related.ID {
		t.Fatalf("related target primary = %#v ok=%v err=%v, want %s", selected, ok, err, related.ID)
	}
	entry, err := service.queries.GetAgentEntry(context.Background(), db.GetAgentEntryParams{LineageID: nextRelated.LineageID, EntryID: nextRelated.HeadEntryID.String})
	if err != nil {
		t.Fatalf("GetAgentEntry() error = %v", err)
	}
	contextText := assistantTextFromPayload(entry.PayloadJson)
	if !strings.Contains(contextText, "<qrspi-context>") || strings.Contains(contextText, "qrspi_result:") {
		t.Fatalf("context starter = %q, want qrspi-context only", contextText)
	}

	freeform, err := service.CreateThreadFromWorkspace(context.Background(), CreateThreadFromWorkspaceInput{
		UserEmail:      "user@example.com",
		SourceThreadID: thread.ID,
		TargetKind:     NewThreadTargetFreeform,
	})
	if err != nil {
		t.Fatalf("CreateThreadFromWorkspace(freeform) error = %v", err)
	}
	if _, ok, err := service.ResolvePrimaryWorkspaceForThread(context.Background(), "user@example.com", freeform.ID); err != nil || ok {
		t.Fatalf("freeform primary ok=%v err=%v, want none", ok, err)
	}
	if freeform.HeadEntryID.Valid {
		t.Fatalf("freeform head = %q, want no context entry", freeform.HeadEntryID.String)
	}
}

func TestBuildThreadMetadataViewListsTargets(t *testing.T) {
	service := newTestAgentChatService(t)
	primary := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	related := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	thread := mustCreateAgentThread(t, service, "thread-metadata", "user@example.com", "/tmp/source", "lineage-metadata")
	if err := service.SetThreadPrimaryWorkspace(context.Background(), thread.ID, primary.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace() error = %v", err)
	}
	if err := service.AddThreadRelatedWorkspace(context.Background(), thread.ID, related.ID, "test"); err != nil {
		t.Fatalf("AddThreadRelatedWorkspace() error = %v", err)
	}
	ctx, err := service.GetThreadWorkspaceContext(context.Background(), "user@example.com", thread.ID)
	if err != nil {
		t.Fatalf("GetThreadWorkspaceContext() error = %v", err)
	}
	view := service.BuildThreadMetadataView(context.Background(), ctx, "/tmp/pi")
	if view.Primary == nil || view.Primary.WorkspaceID != primary.ID {
		t.Fatalf("primary = %#v, want %s", view.Primary, primary.ID)
	}
	if len(view.Related) != 1 || view.Related[0].WorkspaceID != related.ID {
		t.Fatalf("related = %#v, want %s", view.Related, related.ID)
	}
	for _, want := range []string{"/thoughts/?", "context=chat", "thread=" + thread.ID} {
		if !strings.Contains(view.URL, want) {
			t.Fatalf("metadata URL = %q, want %q", view.URL, want)
		}
	}
	if strings.Contains(view.URL, "chat_workspace=") {
		t.Fatalf("metadata URL = %q, want no chat_workspace with thread", view.URL)
	}
	if strings.Contains(view.URL, "/agent-chat") {
		t.Fatalf("metadata URL = %q, should not use retired page route", view.URL)
	}
	kinds := []NewThreadTargetKind{}
	for _, target := range view.NewTargets {
		kinds = append(kinds, target.Kind)
	}
	if got := strings.Trim(strings.Join([]string{string(kinds[0]), string(kinds[1]), string(kinds[2])}, ","), ","); got != "primary,related,freeform" {
		t.Fatalf("target kinds = %s", got)
	}
}

func mustJSONText(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal JSON text: %v", err)
	}
	return string(encoded)
}
