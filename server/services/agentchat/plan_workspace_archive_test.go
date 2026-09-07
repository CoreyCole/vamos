package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
	serverdb "github.com/CoreyCole/vamos/server/services/db"
)

func newPlanArchiveTestService(t *testing.T) (*Service, *db.Queries) {
	t.Helper()
	database, err := serverdb.NewService(filepath.Join(t.TempDir(), "plan-archive.db"))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return &Service{db: database.DB(), queries: database.Queries}, database.Queries
}

func seedPlanWorkspace(t *testing.T, ctx context.Context, q *db.Queries, rel, projectID string) {
	t.Helper()
	if _, err := q.UpsertDiscoveredPlanWorkspace(ctx, db.UpsertDiscoveredPlanWorkspaceParams{
		PlanDirRel:        rel,
		ProjectID:         projectID,
		PlanDir:           "thoughts/" + rel,
		Label:             rel,
		ArtifactUpdatedAt: time.Now(),
		QrspiLifecycle:    "implement",
	}); err != nil {
		t.Fatalf("UpsertDiscoveredPlanWorkspace: %v", err)
	}
}

func TestManualArchiveUnarchiveAndListFilters(t *testing.T) {
	ctx := context.Background()
	service, q := newPlanArchiveTestService(t)
	rel := "agent/plans/archive-api"
	seedPlanWorkspace(t, ctx, q, rel, "vamos")
	seedPlanWorkspace(t, ctx, q, "agent/plans/still-current", "vamos")

	view, err := service.ManualArchivePlanWorkspace(ctx, rel, "corey@example.com")
	if err != nil {
		t.Fatalf("ManualArchivePlanWorkspace: %v", err)
	}
	if view.ArchiveReason != "manual" || view.ArchivedByEmail != "corey@example.com" || view.ArchivedAt == nil {
		t.Fatalf("archive view = %#v", view)
	}

	again, err := service.ManualArchivePlanWorkspace(ctx, rel, "other@example.com")
	if err != nil {
		t.Fatalf("idempotent archive: %v", err)
	}
	if again.ArchivedByEmail != "corey@example.com" || !again.ArchivedAt.Equal(*view.ArchivedAt) {
		t.Fatalf("idempotent archive mutated metadata: %#v", again)
	}

	current, err := service.ListPlanWorkspacesByStatus(ctx, "current", "")
	if err != nil {
		t.Fatalf("list current: %v", err)
	}
	for _, plan := range current.Plans {
		if plan.PlanDirRel == rel {
			t.Fatalf("manual archived plan still listed as current")
		}
	}

	archived, err := service.ListPlanWorkspacesByStatus(ctx, "archived", "")
	if err != nil {
		t.Fatalf("list archived: %v", err)
	}
	if len(archived.Plans) != 1 || archived.Plans[0].PlanDirRel != rel {
		t.Fatalf("archived list = %#v", archived.Plans)
	}

	cleared, err := service.UnarchiveManualPlanWorkspace(ctx, rel)
	if err != nil {
		t.Fatalf("UnarchiveManualPlanWorkspace: %v", err)
	}
	if cleared.ArchivedAt != nil || cleared.ArchiveReason != "" || cleared.ArchivedByEmail != "" {
		t.Fatalf("cleared view = %#v", cleared)
	}
}

func TestUnarchiveMissingFromDiskRefused(t *testing.T) {
	ctx := context.Background()
	service, q := newPlanArchiveTestService(t)
	rel := "agent/plans/missing-disk"
	seedPlanWorkspace(t, ctx, q, rel, "vamos")
	if _, err := q.ArchiveMissingPlanWorkspaces(ctx, []string{"agent/plans/other"}); err != nil {
		t.Fatalf("ArchiveMissingPlanWorkspaces: %v", err)
	}

	_, err := service.UnarchiveManualPlanWorkspace(ctx, rel)
	if !errors.Is(err, ErrPlanWorkspaceMissingFromDiskArchive) {
		t.Fatalf("err = %v, want ErrPlanWorkspaceMissingFromDiskArchive", err)
	}
}

func TestArchiveAPIHandlersHTTP(t *testing.T) {
	ctx := context.Background()
	service, q := newPlanArchiveTestService(t)
	rel := "agent/plans/http-archive"
	seedPlanWorkspace(t, ctx, q, rel, "vamos")
	handler := NewHandler(service, nil)

	e := echo.New()
	req := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/plan-workspaces/archive",
		bytes.NewBufferString(`{"plan_dir_rel":"`+rel+`"}`),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_email", "corey@example.com")
	if err := handler.ArchivePlanWorkspaceAPI(c); err != nil {
		t.Fatalf("ArchivePlanWorkspaceAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("archive status = %d body=%s", rec.Code, rec.Body.String())
	}
	var archived PlanWorkspaceArchiveView
	if err := json.Unmarshal(rec.Body.Bytes(), &archived); err != nil {
		t.Fatalf("decode archive response: %v", err)
	}
	if archived.ArchiveReason != "manual" || archived.ArchivedByEmail != "corey@example.com" {
		t.Fatalf("archive response = %#v", archived)
	}

	// Idempotent second archive.
	req2 := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/plan-workspaces/archive",
		bytes.NewBufferString(`{"plan_dir_rel":"`+rel+`"}`),
	)
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.Set("user_email", "corey@example.com")
	if err := handler.ArchivePlanWorkspaceAPI(c2); err != nil {
		t.Fatalf("idempotent ArchivePlanWorkspaceAPI: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("idempotent archive status = %d", rec2.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/agent-chat/plan-workspaces?status=archived", nil)
	listRec := httptest.NewRecorder()
	listC := e.NewContext(listReq, listRec)
	listC.Set("user_email", "corey@example.com")
	if err := handler.ListPlanWorkspacesAPI(listC); err != nil {
		t.Fatalf("ListPlanWorkspacesAPI: %v", err)
	}
	var listed PlanWorkspaceListResult
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listed.Status != "archived" || len(listed.Plans) != 1 {
		t.Fatalf("listed = %#v", listed)
	}

	unReq := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/plan-workspaces/unarchive",
		bytes.NewBufferString(`{"plan_dir_rel":"`+rel+`"}`),
	)
	unReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	unRec := httptest.NewRecorder()
	unC := e.NewContext(unReq, unRec)
	unC.Set("user_email", "corey@example.com")
	if err := handler.UnarchivePlanWorkspaceAPI(unC); err != nil {
		t.Fatalf("UnarchivePlanWorkspaceAPI: %v", err)
	}

	missingRel := "agent/plans/http-missing"
	seedPlanWorkspace(t, ctx, q, missingRel, "vamos")
	if _, err := q.ArchiveMissingPlanWorkspaces(ctx, []string{rel}); err != nil {
		t.Fatalf("seed missing: %v", err)
	}
	badReq := httptest.NewRequest(
		http.MethodPost,
		"/agent-chat/plan-workspaces/unarchive",
		bytes.NewBufferString(`{"plan_dir_rel":"`+missingRel+`"}`),
	)
	badReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	badRec := httptest.NewRecorder()
	badC := e.NewContext(badReq, badRec)
	badC.Set("user_email", "corey@example.com")
	err := handler.UnarchivePlanWorkspaceAPI(badC)
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusConflict {
		t.Fatalf("missing_from_disk unarchive err = %#v, want 409", err)
	}
}
