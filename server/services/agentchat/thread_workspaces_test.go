package agentchat

import (
	"context"
	"database/sql"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestThreadWorkspacePrimaryPromotionDemotesPreviousPrimary(t *testing.T) {
	service := newTestAgentChatService(t)
	ctx := context.Background()
	thread := mustCreateAgentThread(t, service, "thread-assoc", "user@example.com", "/tmp/project", "lineage-assoc")
	workspaceA := mustCreateThreadWorkspaceTestWorkspace(t, service, "workspace-a", "/tmp/project/a")
	workspaceB := mustCreateThreadWorkspaceTestWorkspace(t, service, "workspace-b", "/tmp/project/b")

	if err := service.SetThreadPrimaryWorkspace(ctx, thread.ID, workspaceA.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace(a): %v", err)
	}
	if err := service.SetThreadPrimaryWorkspace(ctx, thread.ID, workspaceB.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace(b): %v", err)
	}

	primary, err := service.queries.GetPrimaryWorkspaceForThread(ctx, db.GetPrimaryWorkspaceForThreadParams{
		ThreadID:  thread.ID,
		UserEmail: thread.UserEmail,
	})
	if err != nil {
		t.Fatalf("GetPrimaryWorkspaceForThread: %v", err)
	}
	if primary.ID != workspaceB.ID {
		t.Fatalf("primary workspace = %q, want %q", primary.ID, workspaceB.ID)
	}
	primaryCount, err := service.queries.TestSupportCountPrimaryThreadWorkspaceAssociations(ctx, thread.ID)
	if err != nil {
		t.Fatalf("count primary associations: %v", err)
	}
	if primaryCount != 1 {
		t.Fatalf("primary association count = %d, want 1", primaryCount)
	}
}

func TestThreadWorkspaceRelatedUpsertAndRunSessionBackfill(t *testing.T) {
	service := newTestAgentChatService(t)
	ctx := context.Background()
	thread := mustCreateAgentThread(t, service, "thread-related", "user@example.com", "/tmp/project", "lineage-related")
	workspace := mustCreateThreadWorkspaceTestWorkspace(t, service, "workspace-related", "/tmp/project/related")
	run := mustCreateAgentRun(t, service, thread.ID, "run-related")
	if _, err := service.queries.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  "session-related",
		ProjectedThreadID:   nullString(thread.ID),
		IndexedByUserEmail:  nullString(thread.UserEmail),
		IdentityKind:        "web",
		ArtifactPath:        sql.NullString{},
		ExternalSessionID:   sql.NullString{},
		ParentSessionID:     sql.NullString{},
		Cwd:                 nullString(thread.Cwd),
		ProjectionState:     "pending",
		AttachedWorkspaceID: sql.NullString{},
		PlanDir:             sql.NullString{},
		ImportedHeadEntryID: sql.NullString{},
		LastError:           sql.NullString{},
		MetadataJson:        sql.NullString{},
	}); err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}

	if err := service.AddThreadRelatedWorkspace(ctx, thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("AddThreadRelatedWorkspace: %v", err)
	}
	if err := service.AddThreadRelatedWorkspace(ctx, thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("AddThreadRelatedWorkspace second call: %v", err)
	}
	associationCount, err := service.queries.TestSupportCountRelatedThreadWorkspaceAssociation(ctx, db.TestSupportCountRelatedThreadWorkspaceAssociationParams{ThreadID: thread.ID, WorkspaceID: workspace.ID})
	if err != nil {
		t.Fatalf("count related associations: %v", err)
	}
	if associationCount != 1 {
		t.Fatalf("related association count = %d, want 1", associationCount)
	}

	if err := service.SetThreadPrimaryWorkspace(ctx, thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("SetThreadPrimaryWorkspace: %v", err)
	}
	storedRun, err := service.queries.GetAgentRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if !storedRun.WorkspaceID.Valid || storedRun.WorkspaceID.String != workspace.ID {
		t.Fatalf("run workspace = %v, want %s", storedRun.WorkspaceID, workspace.ID)
	}
	sessionWorkspace, err := service.queries.TestSupportGetAgentSessionWorkspaceID(ctx, "session-related")
	if err != nil {
		t.Fatalf("select session workspace: %v", err)
	}
	if !sessionWorkspace.Valid || sessionWorkspace.String != workspace.ID {
		t.Fatalf("session workspace = %v, want %s", sessionWorkspace, workspace.ID)
	}
}

func mustCreateThreadWorkspaceTestWorkspace(t *testing.T, service *Service, id, root string) db.Workspace {
	t.Helper()
	workspace, err := service.queries.CreateWorkspace(context.Background(), db.CreateWorkspaceParams{
		ID:                id,
		UserEmail:         "user@example.com",
		Title:             id,
		RootDocPath:       root,
		Cwd:               nullString(root),
		WorkflowType:      string(WorkspaceWorkflowQRSPI),
		WorkflowStateJson: sql.NullString{},
		Source:            string(WorkspaceSourceWeb),
		SelectedThreadID:  sql.NullString{},
		SelectedDocPath:   sql.NullString{},
	})
	if err != nil {
		t.Fatalf("CreateWorkspace(%s): %v", id, err)
	}
	return workspace
}
