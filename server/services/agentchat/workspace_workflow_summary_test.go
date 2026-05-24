package agentchat

import (
	"database/sql"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestWorkflowSummaryFromWorkspaceParsesRuntimeState(t *testing.T) {
	row := db.Workspace{
		WorkflowType: "qrspi",
		WorkflowStateJson: sql.NullString{Valid: true, String: `{
			"type":"qrspi",
			"current_node_id":"human-review",
			"status":"waiting_human",
			"human_gate":{"from":"verify","to":"human-review","reason":"approval"},
			"last_result":{"outcome":"ready-for-promotion","primary_artifact":"verify.md","display_next":"/approve"}
		}`},
	}

	summary := workflowSummaryFromWorkspace(row)
	if summary.WorkflowType != "qrspi" || summary.Stage != "human-review" || summary.Status != "waiting_human" || !summary.WaitingHuman {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.Outcome != "ready-for-promotion" || summary.PrimaryArtifact != "verify.md" || summary.NextStep != "human-review" {
		t.Fatalf("summary result fields = %+v", summary)
	}
}

func TestWorkflowSummaryFromWorkspaceHandlesMalformedJSON(t *testing.T) {
	summary := workflowSummaryFromWorkspace(db.Workspace{
		WorkflowType:      "qrspi",
		WorkflowStateJson: sql.NullString{Valid: true, String: `{not json`},
	})
	if summary.WorkflowType != "qrspi" || summary.Stage != "unknown" || summary.Status != "unknown" {
		t.Fatalf("summary = %+v", summary)
	}
}
