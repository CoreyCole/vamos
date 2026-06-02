package agentchat

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type recordingPlanWorkspaceNotifier struct{ count int }

func (n *recordingPlanWorkspaceNotifier) NotifyProjectPlanSidebar() WorkspaceStreamSignal {
	n.count++
	return WorkspaceStreamSignal{Scope: PatchWorkspaceSidebar}
}

func TestImplWorkspaceDiscoveryConfigPreservesConfiguredCheckouts(t *testing.T) {
	cfg := workspaces.ImplWorkspaceDiscoveryConfig{
		Domain: "workspaces.test",
		ConfiguredCheckouts: map[string]workspaces.ConfiguredCheckout{
			"work": {RootPath: "/repo/vamos", DisplayName: "Working checkout"},
		},
	}

	normalized := normalizeImplWorkspaceDiscoveryConfig(cfg)
	if normalized.ConfiguredCheckouts["work"].RootPath != "/repo/vamos" {
		t.Fatalf("normalized configured checkouts = %#v", normalized.ConfiguredCheckouts)
	}
	discovery := implDiscoveryAsDiscoveryConfig(normalized)
	if discovery.ConfiguredCheckouts["work"].DisplayName != "Working checkout" {
		t.Fatalf("discovery configured checkouts = %#v", discovery.ConfiguredCheckouts)
	}
}

func TestServiceWorkspaceSyncInputPreservesConfiguredCheckouts(t *testing.T) {
	service := newTestAgentChatService(t)
	service.SetImplWorkspaceDiscoveryConfig(workspaces.ImplWorkspaceDiscoveryConfig{
		ParentDir: "/repo",
		Domain:    "workspaces.test",
		ConfiguredCheckouts: map[string]workspaces.ConfiguredCheckout{
			"stage": {RootPath: "/repo/vamos-stage", DisplayName: "Stage checkout"},
		},
	})

	input := service.WorkspaceSyncInput()
	stage := input.ImplWorkspaces.ConfiguredCheckouts["stage"]
	if stage.RootPath != "/repo/vamos-stage" || stage.DisplayName != "Stage checkout" {
		t.Fatalf("workspace sync configured checkouts = %#v", input.ImplWorkspaces.ConfiguredCheckouts)
	}
}

func TestPlanWorkspaceScannerUsesThoughtsRootNotProjectRoot(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "cn-agents")
	thoughtsRoot := filepath.Join(root, "host-thoughts")
	planDir := filepath.Join(thoughtsRoot, "alice", "plans", "split-root")
	writePlanWorkspaceFile(t, planDir, "plan.md", time.Now())

	_ = projectRoot
	rows, err := (PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot}).Scan(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Scan() discovered %d rows, want 1", len(rows))
	}
	if rows[0].PlanDir != planDir {
		t.Fatalf("PlanDir = %q, want %q", rows[0].PlanDir, planDir)
	}
}

func TestPlanWorkspaceScannerDiscoversExpectedRoots(t *testing.T) {
	thoughtsRoot := t.TempDir()
	oldTime := time.Date(2026, 5, 1, 10, 0, 0, 0, time.Local)
	newTime := time.Date(2026, 5, 2, 10, 0, 0, 0, time.Local)

	top := filepath.Join(thoughtsRoot, "alice", "plans", "foo")
	writePlanWorkspaceFile(t, top, "plan.md", oldTime)
	writePlanWorkspaceFile(t, top, "notes.txt", newTime)
	writePlanWorkspaceFile(t, top, "ignored.bin", newTime.Add(24*time.Hour))
	writePlanWorkspaceFile(
		t,
		filepath.Join(top, ".git"),
		"ignored.md",
		newTime.Add(48*time.Hour),
	)
	writePlanWorkspaceFile(
		t,
		filepath.Join(top, "node_modules"),
		"ignored.md",
		newTime.Add(72*time.Hour),
	)

	reviewPlan := filepath.Join(
		top,
		"reviews",
		"2026-05-17_15-18-02_demo_implementation-review",
	)
	writePlanWorkspaceFile(t, reviewPlan, "design.md", oldTime.Add(time.Hour))
	writePlanWorkspaceFile(t, reviewPlan, "outline.md", oldTime.Add(2*time.Hour))
	writePlanWorkspaceFile(t, reviewPlan, "review.json", oldTime.Add(3*time.Hour))
	writePlanWorkspaceFile(
		t,
		filepath.Join(top, "reviews", "unmarked"),
		"review.md",
		oldTime,
	)
	for _, name := range []string{"context", "questions", "research", "adrs", "handoffs", "prds"} {
		writePlanWorkspaceFile(
			t,
			filepath.Join(top, name),
			"note.md",
			newTime.Add(time.Hour),
		)
		writePlanWorkspaceFile(
			t,
			filepath.Join(reviewPlan, name),
			"note.md",
			newTime.Add(2*time.Hour),
		)
	}
	writePlanWorkspaceFile(
		t,
		filepath.Join(thoughtsRoot, "alice", "notes", "with-agents"),
		"AGENTS.md",
		newTime.Add(time.Hour),
	)

	rows, err := (PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot}).Scan(
		context.Background(),
	)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	gotRels := make([]string, 0, len(rows))
	byRel := map[string]DiscoveredPlanWorkspace{}
	for _, row := range rows {
		gotRels = append(gotRels, row.PlanDirRel)
		byRel[row.PlanDirRel] = row
	}
	wantRels := []string{
		"alice/plans/foo",
		"alice/plans/foo/reviews/2026-05-17_15-18-02_demo_implementation-review",
	}
	if !reflect.DeepEqual(gotRels, wantRels) {
		t.Fatalf("rels = %#v, want %#v", gotRels, wantRels)
	}
	if got := byRel["alice/plans/foo"].ArtifactUpdatedAt; !got.Equal(
		newTime.Add(2 * time.Hour),
	) {
		t.Fatalf("top activity = %s, want %s", got, newTime.Add(2*time.Hour))
	}
	nestedRel := "alice/plans/foo/reviews/2026-05-17_15-18-02_demo_implementation-review"
	if got := byRel[nestedRel].ArtifactUpdatedAt; !got.Equal(newTime.Add(2 * time.Hour)) {
		t.Fatalf("nested activity = %s, want %s", got, newTime.Add(2*time.Hour))
	}
}

func TestPlanWorkspaceRootMarkers(t *testing.T) {
	root := t.TempDir()
	top := filepath.Join(root, "alice", "plans", "foo")
	nested := filepath.Join(top, "reviews", "follow-up")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if !isPlanWorkspaceRoot(root, top) {
		t.Fatal("top-level plan should be root")
	}
	if isPlanWorkspaceRoot(root, nested) {
		t.Fatal("unmarked nested dir should not be root")
	}
	writePlanWorkspaceFile(t, nested, "outline.md", time.Now())
	if !isPlanWorkspaceRoot(root, nested) {
		t.Fatal("nested outline.md should mark root")
	}
}

func TestPlanWorkspaceScannerRejectsMissingThoughtsRoot(t *testing.T) {
	_, err := (PlanWorkspaceScanner{}).Scan(context.Background())
	if err == nil {
		t.Fatal("Scan() error = nil, want required thoughts root error")
	}
}

func TestPlanWorkspaceScannerReadsQRSPILifecycleFrontmatter(t *testing.T) {
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "lifecycle-plan")
	writePlanWorkspaceFile(t, planDir, "plan.md", time.Now())
	if err := os.WriteFile(
		filepath.Join(planDir, "AGENTS.md"),
		[]byte("---\nqrspi_lifecycle: merged\nqrspi_lifecycle_updated_at: 2026-05-24T10:00:00Z\nqrspi_closed_reason: shipped\n---\n# Plan\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	rows, err := (PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot}).Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].QRSPIStage != "merged" || rows[0].QRSPIClosedReason != "shipped" {
		t.Fatalf("lifecycle = %q reason %q", rows[0].QRSPIStage, rows[0].QRSPIClosedReason)
	}
	if rows[0].QRSPILifecycleUpdatedAt.IsZero() {
		t.Fatal("QRSPILifecycleUpdatedAt is zero")
	}
}

func TestPlanWorkspaceScannerDefaultsMissingLifecycleToQuestion(t *testing.T) {
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "default-plan")
	writePlanWorkspaceFile(t, planDir, "plan.md", time.Now())

	rows, err := (PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot}).Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(rows) != 1 || rows[0].QRSPIStage != "question" {
		t.Fatalf("rows = %#v, want question lifecycle", rows)
	}
}

func TestPlanWorkspaceScannerPopulatesProjectFromFrontmatterOrInput(t *testing.T) {
	thoughtsRoot := t.TempDir()
	frontmatterPlan := filepath.Join(thoughtsRoot, "agent", "plans", "frontmatter-project")
	inputPlan := filepath.Join(thoughtsRoot, "agent", "plans", "input-project")
	writePlanWorkspaceFile(t, frontmatterPlan, "plan.md", time.Now())
	writePlanWorkspaceFile(t, inputPlan, "plan.md", time.Now())
	if err := os.WriteFile(
		filepath.Join(frontmatterPlan, "AGENTS.md"),
		[]byte("---\nproject: example.com/alpha/app\n---\n# Plan\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	rows, err := (PlanWorkspaceScanner{
		ThoughtsRoot: thoughtsRoot,
		ProjectID:    "example.com/beta/app",
	}).Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	byRel := map[string]DiscoveredPlanWorkspace{}
	for _, row := range rows {
		byRel[row.PlanDirRel] = row
	}
	if got := byRel["agent/plans/frontmatter-project"].ProjectID; got != "example.com/alpha/app" {
		t.Fatalf("frontmatter project = %q", got)
	}
	if got := byRel["agent/plans/input-project"].ProjectID; got != "example.com/beta/app" {
		t.Fatalf("input project = %q", got)
	}
}

func TestPlanWorkspaceSyncerListsCurrentAndHistoricalLifecycle(t *testing.T) {
	service := newTestAgentChatService(t)
	ctx := context.Background()
	for _, item := range []struct {
		rel   string
		stage string
	}{
		{rel: "agent/plans/current", stage: "implement"},
		{rel: "agent/plans/merged", stage: "merged"},
		{rel: "agent/plans/closed", stage: "closed"},
	} {
		_, err := service.queries.UpsertDiscoveredPlanWorkspace(ctx, db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        item.rel,
			PlanDir:           filepath.Join(service.thoughtsRoot, filepath.FromSlash(item.rel)),
			Label:             item.rel,
			WorkspaceSlug:     item.rel,
			ArtifactUpdatedAt: time.Now(),
			QrspiLifecycle:    item.stage,
		})
		if err != nil {
			t.Fatalf("UpsertDiscoveredPlanWorkspace(%s) error = %v", item.stage, err)
		}
	}
	current, err := service.queries.ListCurrentPlanWorkspaces(ctx, "")
	if err != nil {
		t.Fatalf("ListCurrentPlanWorkspaces() error = %v", err)
	}
	if len(current) != 1 || current[0].QrspiLifecycle != "implement" {
		t.Fatalf("current = %#v, want only implement", current)
	}
	all, err := service.queries.ListPlanWorkspaces(ctx, "")
	if err != nil {
		t.Fatalf("ListPlanWorkspaces() error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all len = %d, want 3", len(all))
	}
}

func TestPlanWorkspaceSyncerPersistsAndFiltersProjectID(t *testing.T) {
	service := newTestAgentChatService(t)
	ctx := context.Background()
	for _, item := range []struct {
		rel     string
		project string
	}{
		{rel: "agent/plans/alpha", project: "example.com/alpha/app"},
		{rel: "agent/plans/beta", project: "example.com/beta/app"},
	} {
		_, err := service.queries.UpsertDiscoveredPlanWorkspace(ctx, db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        item.rel,
			ProjectID:         item.project,
			PlanDir:           filepath.Join(service.thoughtsRoot, filepath.FromSlash(item.rel)),
			Label:             item.rel,
			WorkspaceSlug:     item.rel,
			ArtifactUpdatedAt: time.Now(),
			QrspiLifecycle:    "implement",
		})
		if err != nil {
			t.Fatalf("UpsertDiscoveredPlanWorkspace(%s) error = %v", item.rel, err)
		}
	}

	all, err := service.queries.ListCurrentPlanWorkspaces(ctx, "")
	if err != nil {
		t.Fatalf("ListCurrentPlanWorkspaces(all) error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("all len = %d, want 2", len(all))
	}
	alpha, err := service.queries.ListCurrentPlanWorkspaces(ctx, "example.com/alpha/app")
	if err != nil {
		t.Fatalf("ListCurrentPlanWorkspaces(alpha) error = %v", err)
	}
	if len(alpha) != 1 || alpha[0].ProjectID != "example.com/alpha/app" || alpha[0].PlanDirRel != "agent/plans/alpha" {
		t.Fatalf("alpha rows = %#v", alpha)
	}
}

func TestPlanWorkspaceScannerPopulatesSlugAndImplMetadata(t *testing.T) {
	thoughtsRoot := t.TempDir()
	parent := t.TempDir()
	planName := "2026-05-16_20-48-11_qrspi-auto-mode-workspace-config-ux"
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", planName)
	activityTime := time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local)
	writePlanWorkspaceFile(t, planDir, "plan.md", activityTime)
	implDir := filepath.Join(parent, "cn-agents-"+planName)
	if err := os.MkdirAll(filepath.Join(implDir, "pkg", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(implDir, "pkg", "agents", "go.mod"),
		[]byte("module github.com/CoreyCole/vamos\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	discoveredAt := time.Date(2026, 5, 5, 10, 0, 0, 0, time.Local)

	rows, err := (PlanWorkspaceScanner{
		ThoughtsRoot: thoughtsRoot,
		ImplWorkspaces: workspaces.ImplWorkspaceDiscoveryConfig{
			ParentDir:        parent,
			Domain:           "workspaces.test",
			CheckoutPrefixes: []string{"cn-agents"},
			MainCheckoutName: "cn-agents",
			PackageSubdir:    "pkg/agents",
		},
		Now: func() time.Time { return discoveredAt },
	}).Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d", len(rows))
	}
	row := rows[0]
	wantSlug := "agent-plans-2026-05-16-20-48-11-qrspi-auto-mode-worksp-cc118c9f"
	if row.WorkspaceSlug != wantSlug {
		t.Fatalf("WorkspaceSlug = %q", row.WorkspaceSlug)
	}
	if row.ImplWorkspacePath != implDir {
		t.Fatalf("ImplWorkspacePath = %q want %q", row.ImplWorkspacePath, implDir)
	}
	if row.ImplWorkspaceURL != "https://"+wantSlug+".workspaces.test/" {
		t.Fatalf("ImplWorkspaceURL = %q", row.ImplWorkspaceURL)
	}
	if !row.ImplWorkspaceDiscoveredAt.Equal(discoveredAt) {
		t.Fatalf("ImplWorkspaceDiscoveredAt = %s", row.ImplWorkspaceDiscoveredAt)
	}
}

func TestPlanWorkspaceSyncerClearsMissingImplMetadata(t *testing.T) {
	service := newTestAgentChatService(t)
	thoughtsRoot := t.TempDir()
	parent := t.TempDir()
	planName := "impl-sync-plan"
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", planName)
	writePlanWorkspaceFile(
		t,
		planDir,
		"plan.md",
		time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local),
	)
	implDir := filepath.Join(parent, "cn-agents-"+planName)
	if err := os.MkdirAll(filepath.Join(implDir, "pkg", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(implDir, "pkg", "agents", "go.mod"),
		[]byte("module github.com/CoreyCole/vamos\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	discoveredAt := time.Date(2026, 5, 5, 10, 0, 0, 0, time.Local)
	notifier := &recordingPlanWorkspaceNotifier{}
	syncer := &PlanWorkspaceSyncer{
		Queries: service.queries,
		Scanner: PlanWorkspaceScanner{
			ThoughtsRoot: thoughtsRoot,
			ImplWorkspaces: workspaces.ImplWorkspaceDiscoveryConfig{
				ParentDir:        parent,
				Domain:           "workspaces.test",
				CheckoutPrefixes: []string{"cn-agents"},
				MainCheckoutName: "cn-agents",
				PackageSubdir:    "pkg/agents",
			},
			Now: func() time.Time { return discoveredAt },
		},
		Notifier: notifier,
	}

	if _, err := syncer.Sync(
		context.Background(),
		PlanWorkspaceDiscoveryInput{},
	); err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}
	withImpl, err := service.queries.GetPlanWorkspace(
		context.Background(),
		"agent/plans/impl-sync-plan",
	)
	if err != nil {
		t.Fatalf("GetPlanWorkspace(with impl) error = %v", err)
	}
	if !withImpl.ImplWorkspacePath.Valid || !withImpl.ImplWorkspaceUrl.Valid ||
		!withImpl.ImplWorkspaceDiscoveredAt.Valid {
		t.Fatalf("impl metadata not populated: %#v", withImpl)
	}

	if err := os.RemoveAll(implDir); err != nil {
		t.Fatalf("RemoveAll(impl): %v", err)
	}
	result, err := syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if !result.Changed || notifier.count != 2 {
		t.Fatalf(
			"result = %#v notifier=%d, want changed impl clear",
			result,
			notifier.count,
		)
	}
	withoutImpl, err := service.queries.GetPlanWorkspace(
		context.Background(),
		"agent/plans/impl-sync-plan",
	)
	if err != nil {
		t.Fatalf("GetPlanWorkspace(without impl) error = %v", err)
	}
	if withoutImpl.ImplWorkspacePath.Valid || withoutImpl.ImplWorkspaceUrl.Valid ||
		withoutImpl.ImplWorkspaceDiscoveredAt.Valid {
		t.Fatalf("impl metadata not cleared: %#v", withoutImpl)
	}
	if withoutImpl.ArchivedAt.Valid {
		t.Fatalf("plan row archived after impl deletion: %#v", withoutImpl)
	}
}

func TestPlanWorkspaceSyncerIndexesPlanOwnedAgentSessions(t *testing.T) {
	service := newTestAgentChatService(t)
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "2026-06-02_plan")
	writePlanWorkspaceFile(t, planDir, "plan.md", time.Now())
	sessionPath := filepath.Join(planDir, ".sessions", "pi", "session.jsonl")
	writeSessionHeader(t, sessionPath, `{"type":"session","id":"plan-session","cwd":"/repo","workflow_id":"wf","workflow_node_id":"outline"}`)

	result, err := (&PlanWorkspaceSyncer{
		Queries: service.queries,
		Scanner: PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot},
	}).Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if result.AgentSessionsIndexed != 1 || !result.Changed {
		t.Fatalf("result = %+v, want one indexed session and changed", result)
	}
	row, err := service.queries.GetAgentSessionByPath(context.Background(), nullableString("agent/plans/2026-06-02_plan/.sessions/pi/session.jsonl"))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath(relative): %v", err)
	}
	if row.Agent != "pi" || !row.PlanDir.Valid || row.PlanDir.String != "agent/plans/2026-06-02_plan" || !row.WorkflowID.Valid || row.WorkflowID.String != "wf" || !row.WorkflowNodeID.Valid || row.WorkflowNodeID.String != "outline" || row.ProjectionState != "needs_hydration" || !row.ArtifactPath.Valid || row.ArtifactPath.String != "agent/plans/2026-06-02_plan/.sessions/pi/session.jsonl" {
		t.Fatalf("indexed session = %#v", row)
	}

	service.piSessionsDir = filepath.Join(t.TempDir(), "blocked-global")
	if err := os.Mkdir(service.piSessionsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(service.piSessionsDir, 0o755) }()
	if _, err := (&PlanWorkspaceSyncer{
		Queries: service.queries,
		Scanner: PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot},
	}).Sync(context.Background(), PlanWorkspaceDiscoveryInput{}); err != nil {
		t.Fatalf("Sync should not scan global pi sessions dir: %v", err)
	}
}

func TestPlanWorkspaceSyncerPreservesHydratedProjectionOnUnchangedFile(t *testing.T) {
	service := newTestAgentChatService(t)
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "2026-06-02_plan")
	writePlanWorkspaceFile(t, planDir, "plan.md", time.Now())
	sessionPath := filepath.Join(planDir, ".sessions", "pi", "session.jsonl")
	writeSessionHeader(t, sessionPath, `{"type":"session","id":"plan-session","cwd":"/repo"}`)

	syncer := &PlanWorkspaceSyncer{
		Queries: service.queries,
		Scanner: PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot},
	}
	if _, err := syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{}); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	row, err := service.queries.GetAgentSessionByPath(context.Background(), nullableString("agent/plans/2026-06-02_plan/.sessions/pi/session.jsonl"))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath: %v", err)
	}
	thread := mustCreateAgentThread(t, service, "thread-preserved", "user@example.com", planDir, "lineage-preserved")
	if err := service.queries.UpdateAgentSessionImportFinalState(context.Background(), db.UpdateAgentSessionImportFinalStateParams{
		ID:                  row.ID,
		AttachedWorkspaceID: sql.NullString{},
		ProjectedThreadID:   nullableString(thread.ID),
		ProjectionState:     "hydrated",
		PlanDir:             nullableString("agent/plans/2026-06-02_plan"),
		ImportedHeadEntryID: sql.NullString{},
		MetadataJson:        sql.NullString{},
	}); err != nil {
		t.Fatalf("UpdateAgentSessionImportFinalState: %v", err)
	}

	result, err := syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if result.Changed {
		t.Fatalf("second result = %#v, want unchanged", result)
	}
	row, err = service.queries.GetAgentSessionByPath(context.Background(), nullableString("agent/plans/2026-06-02_plan/.sessions/pi/session.jsonl"))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath: %v", err)
	}
	if row.ProjectionState != "hydrated" || !row.ProjectedThreadID.Valid || row.ProjectedThreadID.String != thread.ID {
		t.Fatalf("row = %#v, want hydrated projection preserved", row)
	}
}

func TestPlanWorkspaceSyncerMarksChangedFileNeedsHydration(t *testing.T) {
	service := newTestAgentChatService(t)
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "2026-06-02_plan")
	writePlanWorkspaceFile(t, planDir, "plan.md", time.Now())
	sessionPath := filepath.Join(planDir, ".sessions", "pi", "session.jsonl")
	writeSessionHeader(t, sessionPath, `{"type":"session","id":"plan-session","cwd":"/repo"}`)

	syncer := &PlanWorkspaceSyncer{
		Queries: service.queries,
		Scanner: PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot},
	}
	if _, err := syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{}); err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	row, err := service.queries.GetAgentSessionByPath(context.Background(), nullableString("agent/plans/2026-06-02_plan/.sessions/pi/session.jsonl"))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath: %v", err)
	}
	thread := mustCreateAgentThread(t, service, "thread-stale", "user@example.com", planDir, "lineage-stale")
	if err := service.queries.UpdateAgentSessionImportFinalState(context.Background(), db.UpdateAgentSessionImportFinalStateParams{
		ID:                  row.ID,
		AttachedWorkspaceID: sql.NullString{},
		ProjectedThreadID:   nullableString(thread.ID),
		ProjectionState:     "hydrated",
		PlanDir:             nullableString("agent/plans/2026-06-02_plan"),
		ImportedHeadEntryID: sql.NullString{},
		MetadataJson:        sql.NullString{},
	}); err != nil {
		t.Fatalf("UpdateAgentSessionImportFinalState: %v", err)
	}
	writeSessionHeader(t, sessionPath, `{"type":"session","id":"plan-session","cwd":"/repo","workflow_node_id":"plan"}`)

	result, err := syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("changed Sync: %v", err)
	}
	if !result.Changed {
		t.Fatalf("changed result = %#v, want changed", result)
	}
	row, err = service.queries.GetAgentSessionByPath(context.Background(), nullableString("agent/plans/2026-06-02_plan/.sessions/pi/session.jsonl"))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath: %v", err)
	}
	if row.ProjectionState != "needs_hydration" || row.ProjectedThreadID.Valid {
		t.Fatalf("row = %#v, want changed file reset for hydration", row)
	}
}

func TestPlanWorkspaceSyncerIdempotencyArchiveAndRestore(t *testing.T) {
	service := newTestAgentChatService(t)
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "sync-plan")
	activityTime := time.Date(2026, 5, 3, 9, 0, 0, 0, time.Local)
	writePlanWorkspaceFile(t, planDir, "plan.md", activityTime)

	notifier := &recordingPlanWorkspaceNotifier{}
	syncer := &PlanWorkspaceSyncer{
		Queries:  service.queries,
		Scanner:  PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot},
		Notifier: notifier,
	}

	result, err := syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}
	if !result.Changed || result.Discovered != 1 || result.Upserted != 1 ||
		notifier.count != 1 {
		t.Fatalf(
			"first result = %#v notifier=%d, want changed insert notify",
			result,
			notifier.count,
		)
	}
	if countWorkspaces(t, service) != 0 {
		t.Fatalf(
			"workspaces count = %d, want no discovery-created workspaces",
			countWorkspaces(t, service),
		)
	}
	firstRow, err := service.queries.GetPlanWorkspace(
		context.Background(),
		"agent/plans/sync-plan",
	)
	if err != nil {
		t.Fatalf("GetPlanWorkspace() error = %v", err)
	}

	result, err = syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if result.Changed || notifier.count != 1 {
		t.Fatalf(
			"second result = %#v notifier=%d, want unchanged/no notify",
			result,
			notifier.count,
		)
	}

	if err := os.RemoveAll(planDir); err != nil {
		t.Fatalf("RemoveAll(plan): %v", err)
	}
	result, err = syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("archive Sync() error = %v", err)
	}
	if !result.Changed || result.Archived != 1 || notifier.count != 2 {
		t.Fatalf(
			"archive result = %#v notifier=%d, want archive notify",
			result,
			notifier.count,
		)
	}
	archived, err := service.queries.GetPlanWorkspace(
		context.Background(),
		"agent/plans/sync-plan",
	)
	if err != nil {
		t.Fatalf("GetPlanWorkspace(archived) error = %v", err)
	}
	if !archived.ArchivedAt.Valid {
		t.Fatalf("ArchivedAt invalid after archive: %#v", archived)
	}

	writePlanWorkspaceFile(t, planDir, "plan.md", activityTime)
	result, err = syncer.Sync(context.Background(), PlanWorkspaceDiscoveryInput{})
	if err != nil {
		t.Fatalf("restore Sync() error = %v", err)
	}
	if !result.Changed || result.Restored != 1 || notifier.count != 3 {
		t.Fatalf(
			"restore result = %#v notifier=%d, want restore notify",
			result,
			notifier.count,
		)
	}
	restored, err := service.queries.GetPlanWorkspace(
		context.Background(),
		"agent/plans/sync-plan",
	)
	if err != nil {
		t.Fatalf("GetPlanWorkspace(restored) error = %v", err)
	}
	if restored.ArchivedAt.Valid {
		t.Fatalf("ArchivedAt valid after restore: %#v", restored)
	}
	if !restored.DiscoveredAt.Equal(firstRow.DiscoveredAt) {
		t.Fatalf(
			"DiscoveredAt = %s, want preserved %s",
			restored.DiscoveredAt,
			firstRow.DiscoveredAt,
		)
	}
}

func TestPlanWorkspaceSyncerUsesInputThoughtsRoot(t *testing.T) {
	service := newTestAgentChatService(t)
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "input-root")
	writePlanWorkspaceFile(
		t,
		planDir,
		"plan.md",
		time.Date(2026, 5, 4, 9, 0, 0, 0, time.Local),
	)

	result, err := (&PlanWorkspaceSyncer{Queries: service.queries}).Sync(
		context.Background(),
		PlanWorkspaceDiscoveryInput{ThoughtsRoot: thoughtsRoot},
	)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !result.Changed || result.Discovered != 1 {
		t.Fatalf("result = %#v, want discovered from input thoughts root", result)
	}
}

func writePlanWorkspaceFile(t *testing.T, dir, name string, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(%s): %v", path, err)
	}
	return path
}

func countWorkspaces(t *testing.T, service *Service) int {
	t.Helper()
	count, err := service.queries.TestSupportCountWorkspaces(context.Background())
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("count workspaces: %v", err)
	}
	return int(count)
}

var (
	_ PlanWorkspaceNotifier = (*recordingPlanWorkspaceNotifier)(nil)
	_                       = db.PlanWorkspace{}
)
