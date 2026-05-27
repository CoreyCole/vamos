package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestCreateNextQRSPIThreadUsesImplementationWorkspace(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	implementationWorkspace := t.TempDir()
	thread := mustCreateAgentThread(t, service, "thread-1", "user@example.com", "/home/ruby/cn/chestnut-flake/vamos", "lineage-1")
	resultXML := `<qrspi-result>
  <stage>workspace</stage>
  <status>complete</status>
  <outcome>complete</outcome>
  <workspaceMetadata>
    <planWorkspace>/tmp/plan</planWorkspace>
    <implementationWorkspace>` + implementationWorkspace + `</implementationWorkspace>
    <trunkBranch>main</trunkBranch>
    <stackBottomBranch>cc/base</stackBottomBranch>
    <parentBranch>cc/base</parentBranch>
    <currentBranch>cc/current</currentBranch>
  </workspaceMetadata>
  <policy><autoMode>false</autoMode><enablePlanReviews>true</enablePlanReviews><invalidResultRetryLimit>1</invalidResultRetryLimit></policy>
  <summary><plan-goal>goal</plan-goal><stage-completed>done</stage-completed><key-decisions>next</key-decisions></summary>
  <artifact>/tmp/plan/plan.md</artifact>
  <next><step>Read q-implement</step><step>Start /q-implement</step></next>
</qrspi-result>`
	mustCreateAgentEntry(t, service, thread.LineageID, "assistant-1", "", "message", 1, `{"type":"message","id":"assistant-1","message":{"role":"assistant","content":`+mustJSONText(t, resultXML)+`}}`)
	if err := service.queries.UpdateAgentThreadHead(context.Background(), db.UpdateAgentThreadHeadParams{ID: thread.ID, HeadEntryID: sql.NullString{String: "assistant-1", Valid: true}}); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}
	if err := service.SetThreadPrimaryWorkspace(context.Background(), thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace() error = %v", err)
	}

	next, err := service.CreateNextQRSPIThread(context.Background(), "user@example.com", thread.ID)
	if err != nil {
		t.Fatalf("CreateNextQRSPIThread() error = %v", err)
	}
	if next.Cwd != implementationWorkspace {
		t.Fatalf("next cwd = %q, want implementation workspace %q", next.Cwd, implementationWorkspace)
	}
	primary, ok, err := service.ResolvePrimaryWorkspaceForThread(context.Background(), "user@example.com", next.ID)
	if err != nil || !ok || primary.ID != workspace.ID {
		t.Fatalf("next primary = %#v ok=%v err=%v, want %s", primary, ok, err, workspace.ID)
	}
	entry, err := service.queries.GetAgentEntry(context.Background(), db.GetAgentEntryParams{LineageID: next.LineageID, EntryID: next.HeadEntryID.String})
	if err != nil {
		t.Fatalf("GetAgentEntry() error = %v", err)
	}
	contextText := assistantTextFromPayload(entry.PayloadJson)
	if !strings.Contains(contextText, resultXML) || !strings.Contains(contextText, implementationWorkspace) {
		t.Fatalf("context payload missing prior XML/cwd: %s", entry.PayloadJson)
	}
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
	if !strings.Contains(contextText, "<qrspi-context>") || strings.Contains(contextText, "<qrspi-result>") {
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
	for _, want := range []string{"/thoughts/?", "context=chat", "chat_workspace=" + primary.ID, "thread=" + thread.ID} {
		if !strings.Contains(view.URL, want) {
			t.Fatalf("metadata URL = %q, want %q", view.URL, want)
		}
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
