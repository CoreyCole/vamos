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

func mustJSONText(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal JSON text: %v", err)
	}
	return string(encoded)
}
