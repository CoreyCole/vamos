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
	var count int
	if err := service.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM workspaces").
		Scan(&count); err != nil &&
		err != sql.ErrNoRows {
		t.Fatalf("count workspaces: %v", err)
	}
	return count
}

var (
	_ PlanWorkspaceNotifier = (*recordingPlanWorkspaceNotifier)(nil)
	_                       = db.PlanWorkspace{}
)
