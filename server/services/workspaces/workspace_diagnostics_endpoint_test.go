package workspaces

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
)

type fakeWorkspaceSyncDiagnosticGetter struct {
	row  db.WorkspaceSyncDiagnostic
	rows []db.WorkspaceSyncDiagnostic
	err  error
}

func (f fakeWorkspaceSyncDiagnosticGetter) GetWorkspaceSyncDiagnostic(_ context.Context, _ db.GetWorkspaceSyncDiagnosticParams) (db.WorkspaceSyncDiagnostic, error) {
	if f.err != nil {
		return db.WorkspaceSyncDiagnostic{}, f.err
	}
	if f.row.SyncKind == "" {
		return db.WorkspaceSyncDiagnostic{}, sql.ErrNoRows
	}
	return f.row, nil
}

func (f fakeWorkspaceSyncDiagnosticGetter) ListWorkspaceSyncDiagnostics(_ context.Context, _ string) ([]db.WorkspaceSyncDiagnostic, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(f.rows) > 0 {
		return append([]db.WorkspaceSyncDiagnostic(nil), f.rows...), nil
	}
	if f.row.SyncKind != "" {
		return []db.WorkspaceSyncDiagnostic{f.row}, nil
	}
	return nil, nil
}

func TestWorkspacesPageRendersSourceLabeledDiagnostics(t *testing.T) {
	finished := time.Date(2026, 6, 14, 16, 30, 0, 0, time.UTC)
	views := BuildImplWorkspaceViews(
		[]db.ImplWorkspace{{
			ProjectID:     "github.com/coreycole/vamos",
			WorkspaceSlug: "feature",
			CheckoutPath:  "/repo/feature",
			DisplayName:   "Feature Workspace",
			Status:        string(ImplWorkspaceStatusMerged),
		}},
		[]WorkspaceLifecycleSnapshot{snapshotFromState(
			Workspace{ProjectID: "github.com/coreycole/vamos", Slug: "feature", CheckoutPath: "/repo/feature", Status: StatusCrashed},
			WorkspaceLifecycleState{DesiredState: WorkspaceDesiredRunning, ObservedState: WorkspaceObservedCrashed},
		)},
		WorkspaceLifecycleSnapshot{},
		WithWorkspaceSyncDiagnostic(WorkspaceSyncDiagnostic{
			LastFinishedAt: finished,
			Status:         "ok",
			Warnings: []WorkspaceDiagnostic{{
				Source:        WorkspaceDiagnosticSourceSync,
				Severity:      WorkspaceDiagnosticWarning,
				Code:          "merge_proof_unknown",
				Message:       "Scheduled sync could not prove merge state.",
				ProjectID:     "github.com/coreycole/vamos",
				WorkspaceSlug: "feature",
				CheckoutPath:  "/repo/feature",
			}},
		}),
	)

	var body strings.Builder
	if err := WorkspacesPage(views, "https://main.test", true).Render(t.Context(), &body); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	html := body.String()
	for _, want := range []string{
		"Manager lifecycle: Merged",
		"source: manager DB",
		"Scheduled sync: ok; last finished Jun 14, 16:30; warnings: 1",
		"Local runtime diagnostics: crashed",
		"source: .vamos/run/status.json; diagnostic only",
		"Scheduled sync could not prove merge state.",
		"Local runtime diagnostics may be stale for this non-active workspace.",
		"Cleanup requires human approval. Do not clean up or delete this checkout unless explicitly approved.",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("WorkspacesPage missing %q: %s", want, html)
		}
	}
}

func TestHandleWorkspaceDiagnosticsAttachesLifecycleDiagnostic(t *testing.T) {
	handler := newDiagnosticsHandlerForTest(t)
	handler.implWorkspaces = fakeImplWorkspaceLister{rows: []db.ImplWorkspace{{
		ProjectID:     "github.com/coreycole/vamos",
		WorkspaceSlug: "demo",
		CheckoutPath:  handler.verifier.Manager.List()[0].CheckoutPath,
		Status:        string(ImplWorkspaceStatusMerged),
	}}}
	handler.workspaceSyncDiagnostics = fakeWorkspaceSyncDiagnosticGetter{row: db.WorkspaceSyncDiagnostic{
		ProjectID:    "github.com/coreycole/vamos",
		SyncKind:     "impl_workspaces",
		StartedAt:    time.Date(2026, 6, 14, 16, 29, 0, 0, time.UTC),
		FinishedAt:   sql.NullTime{Time: time.Date(2026, 6, 14, 16, 30, 0, 0, time.UTC), Valid: true},
		Status:       "ok",
		WarningsJson: "[]",
	}}

	rec, err := runWorkspaceDiagnosticsRequest(handler, "secret", "demo", "tail=1&project_id=github.com/coreycole/vamos")
	if err != nil {
		t.Fatalf("HandleWorkspaceDiagnostics: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var diagnostics WorkspaceDiagnostics
	if err := json.Unmarshal(rec.Body.Bytes(), &diagnostics); err != nil {
		t.Fatalf("Unmarshal diagnostics: %v", err)
	}
	if diagnostics.LifecycleDiagnostic == nil {
		t.Fatal("missing lifecycle diagnostic")
	}
	if diagnostics.LifecycleDiagnostic.Lifecycle != ImplWorkspaceStatusMerged || diagnostics.LifecycleDiagnostic.LifecycleSource != WorkspaceDiagnosticSourceManagerDB {
		t.Fatalf("lifecycle diagnostic = %#v", diagnostics.LifecycleDiagnostic)
	}
	if diagnostics.LifecycleDiagnostic.Sync.Status != "ok" {
		t.Fatalf("sync status = %q, want ok", diagnostics.LifecycleDiagnostic.Sync.Status)
	}
}

func TestHandleWorkspaceDiagnosticsOmitsLifecycleDiagnosticWithoutProject(t *testing.T) {
	handler := newDiagnosticsHandlerForTest(t)
	handler.implWorkspaces = fakeImplWorkspaceLister{rows: []db.ImplWorkspace{{
		ProjectID:     "github.com/coreycole/vamos",
		WorkspaceSlug: "demo",
		Status:        string(ImplWorkspaceStatusMerged),
	}}}

	rec, err := runWorkspaceDiagnosticsRequest(handler, "secret", "demo", "tail=1")
	if err != nil {
		t.Fatalf("HandleWorkspaceDiagnostics: %v", err)
	}
	var diagnostics WorkspaceDiagnostics
	if err := json.Unmarshal(rec.Body.Bytes(), &diagnostics); err != nil {
		t.Fatalf("Unmarshal diagnostics: %v", err)
	}
	if diagnostics.LifecycleDiagnostic != nil {
		t.Fatalf("lifecycle diagnostic = %#v, want nil", diagnostics.LifecycleDiagnostic)
	}
}
