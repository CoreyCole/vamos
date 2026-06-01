package agentchat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	_ "modernc.org/sqlite"

	conversation "github.com/CoreyCole/vamos/pkg/agents/conversation"
	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

func TestApplyCheckpointPersistsEntriesAndAdvancesHead(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")

	cp := conversation.Checkpoint{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		HeadEntryID: "assistant-1",
		TurnIndex:   1,
		Header:      conversation.SnapshotHeader{SessionID: thread.ID, Cwd: thread.Cwd},
		NewEntries: []conversation.SnapshotEntry{
			{
				LineageID:   thread.LineageID,
				EntryID:     "model-1",
				EntryType:   "model_change",
				Timestamp:   time.Now().UTC(),
				OriginOrder: 0,
				PayloadJSON: `{"type":"model_change","id":"model-1","parentId":null,"timestamp":"2026-04-19T12:00:00Z","provider":"openai-codex","modelId":"gpt-5.4"}`,
			},
			{
				LineageID:     thread.LineageID,
				EntryID:       "assistant-1",
				ParentEntryID: "model-1",
				EntryType:     "message",
				Timestamp:     time.Now().UTC(),
				OriginOrder:   1,
				PayloadJSON:   `{"type":"message","id":"assistant-1","parentId":"model-1","timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":"done"}}`,
			},
		},
	}

	if err := service.ApplyCheckpoint(t.Context(), cp); err != nil {
		t.Fatalf("ApplyCheckpoint() error = %v", err)
	}

	updated, err := service.queries.GetAgentThread(t.Context(), thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	if !updated.HeadEntryID.Valid || updated.HeadEntryID.String != "assistant-1" {
		t.Fatalf("HeadEntryID = %v, want assistant-1", updated.HeadEntryID)
	}
}

const projectMainPath = "project/main.go"

func TestBuildPlanSidebarTreeNestedPlanDirs(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	projectPlan := filepath.Join(
		service.thoughtsRoot,
		"creative-mode-agent",
		"plans",
		"2026-05-01_project",
	)
	milestonePlan := filepath.Join(projectPlan, "milestones", "m1")
	ticketPlan := filepath.Join(milestonePlan, "tickets", "ticket-a")

	nodes := service.buildPlanSidebarTree([]PlanSidebarSource{
		{
			PlanDir:   ticketPlan,
			Source:    "session",
			SessionID: "session-1",
			UpdatedAt: time.Now(),
		},
	}, "")

	if len(nodes) != 1 || nodes[0].PlanDir != projectPlan {
		t.Fatalf("root nodes = %#v, want project plan root", nodes)
	}
	if len(nodes[0].Children) != 1 ||
		nodes[0].Children[0].PlanDir != filepath.Join(projectPlan, "milestones") {
		t.Fatalf("project children = %#v, want milestones ancestor", nodes[0].Children)
	}
	milestones := nodes[0].Children[0]
	if len(milestones.Children) != 1 || milestones.Children[0].PlanDir != milestonePlan {
		t.Fatalf("milestones children = %#v, want milestone plan", milestones.Children)
	}
	milestone := milestones.Children[0]
	if len(milestone.Children) != 1 ||
		milestone.Children[0].PlanDir != filepath.Join(milestonePlan, "tickets") {
		t.Fatalf("milestone children = %#v, want tickets ancestor", milestone.Children)
	}
	if got := milestone.Children[0].Children[0].PlanDir; got != ticketPlan {
		t.Fatalf("ticket plan = %q, want %q", got, ticketPlan)
	}
}

func TestBuildPlanSidebarTreeActiveAncestorExpansion(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	root := filepath.Join(service.thoughtsRoot, "user", "plans", "project")
	child := filepath.Join(root, "tickets", "ticket-a")

	nodes := service.buildPlanSidebarTree([]PlanSidebarSource{
		{
			PlanDir:   root,
			Source:    "session",
			SessionID: "root-session",
			UpdatedAt: time.Now(),
		},
		{
			PlanDir:   child,
			Source:    "session",
			SessionID: "child-session",
			UpdatedAt: time.Now(),
		},
	}, child)

	if len(nodes) != 1 || !nodes[0].Expanded {
		t.Fatalf("root = %#v, want expanded ancestor", nodes)
	}
	if !nodes[0].Children[0].Expanded || !nodes[0].Children[0].Children[0].Active {
		t.Fatalf("tree = %#v, want active child and expanded ancestors", nodes[0])
	}
}

func TestBuildPlanSidebarStateResolvesActiveThreadPlan(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "active-thread")
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-active-plan",
		"user@example.com",
		planDir,
		"lineage-active-plan",
	)

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com", ActiveThreadID: thread.ID},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if state.ActivePlanDir != planDir {
		t.Fatalf("ActivePlanDir = %q, want %q", state.ActivePlanDir, planDir)
	}
	if len(state.Nodes) != 1 || !state.Nodes[0].Active || !state.Nodes[0].Expanded {
		t.Fatalf("nodes = %#v, want active expanded plan node", state.Nodes)
	}
}

func TestBuildPlanSidebarTreeDirectAndAggregateTimestamps(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	root := filepath.Join(service.thoughtsRoot, "user", "plans", "project")
	child := filepath.Join(root, "tickets", "ticket-a")
	oldTime := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)

	nodes := service.buildPlanSidebarTree([]PlanSidebarSource{
		{PlanDir: root, Source: "session", SessionID: "root-session", UpdatedAt: oldTime},
		{
			PlanDir:   child,
			Source:    "session",
			SessionID: "child-session",
			UpdatedAt: newTime,
		},
	}, child)

	rootNode := nodes[0]
	if !rootNode.DirectLatestAt.Equal(oldTime) ||
		!rootNode.AggregateLatestAt.Equal(newTime) {
		t.Fatalf(
			"timestamps = direct %s aggregate %s, want %s/%s",
			rootNode.DirectLatestAt,
			rootNode.AggregateLatestAt,
			oldTime,
			newTime,
		)
	}
	if !planSidebarTimestamp(rootNode).Equal(oldTime) {
		t.Fatalf(
			"expanded timestamp = %s, want direct %s",
			planSidebarTimestamp(rootNode),
			oldTime,
		)
	}
	rootNode.Expanded = false
	if !planSidebarTimestamp(rootNode).Equal(newTime) {
		t.Fatalf(
			"collapsed timestamp = %s, want aggregate %s",
			planSidebarTimestamp(rootNode),
			newTime,
		)
	}
}

func TestBuildPlanSidebarStateUsesPlanWorkspaceHrefForUnattachedSession(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "unattached-session")
	mustCreateAgentSession(
		t,
		service,
		"session-unattached",
		"user@example.com",
		planDir,
		"",
		"",
	)

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 1 {
		t.Fatalf("nodes = %#v, want one plan node", state.Nodes)
	}
	want := thoughtsDocRedirectURLForRoot(service.thoughtsRoot, planDir, nil)
	if state.Nodes[0].Href != want {
		t.Fatalf("Href = %q, want %q", state.Nodes[0].Href, want)
	}
}

func TestBuildPlanSidebarStateFiltersOutsideThoughtsRoot(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	inside := filepath.Join(service.thoughtsRoot, "user", "plans", "inside")
	outside := filepath.Join(t.TempDir(), "thoughts", "user", "plans", "outside")
	mustCreateAgentSession(
		t,
		service,
		"inside-session",
		"user@example.com",
		inside,
		"",
		"",
	)
	mustCreateAgentSession(
		t,
		service,
		"outside-session",
		"user@example.com",
		outside,
		"",
		"",
	)

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 1 || state.Nodes[0].PlanDir != inside {
		t.Fatalf("nodes = %#v, want only inside plan", state.Nodes)
	}
}

func TestBuildPlanSidebarStateEmptySources(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 0 || state.DrawerTitle == "" || state.TargetID == "" {
		t.Fatalf("state = %#v, want empty initialized state", state)
	}
}

func TestBuildPlanSidebarStateIncludesDiscoveredPlanWorkspacesForAnyUser(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "other-user", "plans", "shared-plan")
	artifactTime := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	mustUpsertDiscoveredPlanWorkspace(t, service, planDir, "Shared Plan", artifactTime)

	for _, userEmail := range []string{"alice@example.com", "bob@example.com"} {
		state, err := service.BuildPlanSidebarState(
			t.Context(),
			PlanSidebarInput{UserEmail: userEmail},
		)
		if err != nil {
			t.Fatalf("BuildPlanSidebarState(%q) error = %v", userEmail, err)
		}
		if len(state.Nodes) != 1 || state.Nodes[0].PlanDir != planDir {
			t.Fatalf("nodes for %q = %#v, want discovered plan", userEmail, state.Nodes)
		}
		wantHref := thoughtsDocRedirectURLForRoot(service.thoughtsRoot, planDir, nil)
		if state.Nodes[0].Href != wantHref {
			t.Fatalf(
				"Href for %q = %q, want %q",
				userEmail,
				state.Nodes[0].Href,
				wantHref,
			)
		}
		if state.Nodes[0].LatestSourceLabel != "Artifact" {
			t.Fatalf(
				"LatestSourceLabel = %q, want Artifact",
				state.Nodes[0].LatestSourceLabel,
			)
		}
	}
}

func TestBuildPlanSidebarStateFiltersByProject(t *testing.T) {
	service := newTestAgentChatService(t)
	now := time.Now().UTC()
	alphaPlan := filepath.Join(service.thoughtsRoot, "user", "plans", "alpha")
	betaPlan := filepath.Join(service.thoughtsRoot, "user", "plans", "beta")
	mustWriteFile(t, filepath.Join(alphaPlan, "AGENTS.md"), "---\nproject: example.com/alpha/app\n---\n# Alpha\n")
	mustWriteFile(t, filepath.Join(betaPlan, "AGENTS.md"), "---\nproject: example.com/beta/app\n---\n# Beta\n")
	mustUpsertDiscoveredPlanWorkspaceWithProject(t, service, alphaPlan, "Alpha", "example.com/alpha/app", now)
	mustUpsertDiscoveredPlanWorkspaceWithProject(t, service, betaPlan, "Beta", "example.com/beta/app", now.Add(time.Minute))
	mustCreateAgentSession(t, service, "alpha-session", "user@example.com", alphaPlan, "", "")
	mustCreateAgentSession(t, service, "beta-session", "user@example.com", betaPlan, "", "")

	allState, err := service.BuildPlanSidebarState(t.Context(), PlanSidebarInput{UserEmail: "user@example.com"})
	if err != nil {
		t.Fatalf("BuildPlanSidebarState(all) error = %v", err)
	}
	if got := flattenPlanSidebarNodeLabels(allState.Nodes); !reflect.DeepEqual(got, []string{"beta", "alpha"}) {
		t.Fatalf("all labels = %#v, want beta and alpha", got)
	}

	alphaState, err := service.BuildPlanSidebarState(t.Context(), PlanSidebarInput{UserEmail: "user@example.com", ProjectID: "example.com/alpha/app"})
	if err != nil {
		t.Fatalf("BuildPlanSidebarState(alpha) error = %v", err)
	}
	if got := flattenPlanSidebarNodeLabels(alphaState.Nodes); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("alpha labels = %#v, want only alpha", got)
	}
}

func TestBuildPlanSidebarStateShowsRelatedProjectBindingStatus(t *testing.T) {
	service := newTestAgentChatService(t)
	now := time.Now().UTC()
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "multi")
	mustWriteFile(t, filepath.Join(planDir, "plan.md"), "---\nproject: vamos\nrelated_projects: [datastarui]\n---\n# Multi\n")
	row := mustUpsertDiscoveredPlanWorkspaceWithProject(t, service, planDir, "Multi", "vamos", now)
	for _, role := range []db.UpsertPlanWorkspaceProjectParams{
		{PlanDirRel: row.PlanDirRel, ProjectID: "vamos", Role: "primary", DeclaredSource: "plan.md"},
		{PlanDirRel: row.PlanDirRel, ProjectID: "datastarui", Role: "related", DeclaredSource: "plan.md"},
	} {
		if _, err := service.queries.UpsertPlanWorkspaceProject(t.Context(), role); err != nil {
			t.Fatalf("UpsertPlanWorkspaceProject() error = %v", err)
		}
	}
	if _, err := service.queries.UpsertPlanWorkspaceImplBinding(t.Context(), db.UpsertPlanWorkspaceImplBindingParams{
		PlanDirRel:    row.PlanDirRel,
		ProjectID:     "datastarui",
		Status:        "planned",
		BindingSource: "metadata",
	}); err != nil {
		t.Fatalf("UpsertPlanWorkspaceImplBinding() error = %v", err)
	}

	state, err := service.BuildPlanSidebarState(t.Context(), PlanSidebarInput{UserEmail: "user@example.com", ProjectID: "datastarui"})
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(state.Nodes))
	}
	node := state.Nodes[0]
	if node.PrimaryProject != "vamos" || !reflect.DeepEqual(node.RelatedProjects, []string{"datastarui"}) || node.MatchedRole != "related" {
		t.Fatalf("node projects = primary %q related %#v matched %q", node.PrimaryProject, node.RelatedProjects, node.MatchedRole)
	}
	if len(node.Bindings) != 1 || node.Bindings[0].ProjectID != "datastarui" || node.Bindings[0].Status != "planned" {
		t.Fatalf("node bindings = %#v, want datastarui planned", node.Bindings)
	}
}

func TestAdoptThreadProjectForRunUsesXMLBeforeFrontmatterWrites(t *testing.T) {
	service := newTestAgentChatService(t)
	alphaPlan := filepath.Join(service.thoughtsRoot, "user", "plans", "alpha")
	mustWriteFile(t, filepath.Join(alphaPlan, "AGENTS.md"), "---\nproject: example.com/alpha/app\n---\n# Alpha\n")
	thread := mustCreateAgentThread(t, service, "project-thread-xml", "user@example.com", service.projectRoot, "lineage-project-xml")
	entries := []db.AgentEntry{
		{EntryID: "call-1", PayloadJson: `{"type":"message","id":"call-1","message":{"role":"assistant","content":[{"type":"toolCall","id":"write-1","name":"write","arguments":{"path":"` + filepath.ToSlash(filepath.Join(alphaPlan, "plan.md")) + `"}}]}}`},
		{EntryID: "tool-1", PayloadJson: `{"type":"message","id":"tool-1","message":{"role":"toolResult","toolCallId":"write-1","toolName":"write","content":"Wrote file","isError":false}}`},
	}
	assistantText := `<qrspi-result>
  <project>example.com/beta/app</project>
  <stage>plan</stage>
  <status>complete</status>
  <outcome>complete</outcome>
  <summary>
    <plan-goal>Test project adoption.</plan-goal>
    <stage-completed>Done.</stage-completed>
    <key-decisions>XML wins.</key-decisions>
  </summary>
</qrspi-result>`

	projectID, err := service.AdoptThreadProjectForRun(t.Context(), thread, entries, assistantText)
	if err != nil {
		t.Fatalf("AdoptThreadProjectForRun() error = %v", err)
	}
	if projectID != "example.com/beta/app" {
		t.Fatalf("projectID = %q, want XML project", projectID)
	}
	updated, err := service.queries.GetAgentThread(t.Context(), thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	if updated.ProjectID != "example.com/beta/app" {
		t.Fatalf("thread project = %q, want beta", updated.ProjectID)
	}
}

func TestAdoptThreadProjectForRunUsesSuccessfulFrontmatterWrites(t *testing.T) {
	service := newTestAgentChatService(t)
	alphaPlan := filepath.Join(service.thoughtsRoot, "user", "plans", "alpha-write")
	mustWriteFile(t, filepath.Join(alphaPlan, "AGENTS.md"), "# Alpha\n")
	mustWriteFile(t, filepath.Join(alphaPlan, "design.md"), "---\nproject: example.com/alpha/app\nstage: design\n---\n# Design\n")
	thread := mustCreateAgentThread(t, service, "project-thread-write", "user@example.com", service.projectRoot, "lineage-project-write")
	entries := []db.AgentEntry{
		{EntryID: "call-1", PayloadJson: `{"type":"message","id":"call-1","message":{"role":"assistant","content":[{"type":"toolCall","id":"write-1","name":"write","arguments":{"path":"` + filepath.ToSlash(filepath.Join(alphaPlan, "design.md")) + `"}}]}}`},
		{EntryID: "tool-1", PayloadJson: `{"type":"message","id":"tool-1","message":{"role":"toolResult","toolCallId":"write-1","toolName":"write","content":"Wrote file","isError":false}}`},
	}

	projectID, err := service.AdoptThreadProjectForRun(t.Context(), thread, entries, "no xml here")
	if err != nil {
		t.Fatalf("AdoptThreadProjectForRun() error = %v", err)
	}
	if projectID != "example.com/alpha/app" {
		t.Fatalf("projectID = %q, want frontmatter project", projectID)
	}
}

func TestAdoptThreadProjectForRunIgnoresCwdDefault(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(t, service, "project-thread-empty", "user@example.com", service.projectRoot, "lineage-project-empty")
	projectID, err := service.AdoptThreadProjectForRun(t.Context(), thread, nil, "no xml here")
	if err != nil {
		t.Fatalf("AdoptThreadProjectForRun() error = %v", err)
	}
	if projectID != "" {
		t.Fatalf("projectID = %q, want no cwd/default adoption", projectID)
	}
}

func TestBuildPlanSidebarStateOverlaysCurrentUserThreadMetadata(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "overlay-plan")
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Overlay Plan",
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	workspace, err := service.CreateWorkspace(
		t.Context(),
		WorkspaceCreateInput{
			UserEmail:   "user@example.com",
			Title:       "Overlay Workspace",
			RootDocPath: planDir,
			Cwd:         planDir,
			Source:      WorkspaceSourceWeb,
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	thread := mustCreateAgentThread(
		t,
		service,
		"overlay-thread",
		"user@example.com",
		service.projectRoot,
		"overlay-lineage",
	)
	if err := service.SetThreadPrimaryWorkspace(t.Context(), thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("attach thread: %v", err)
	}

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 1 {
		t.Fatalf("nodes = %#v, want one merged plan node", state.Nodes)
	}
	node := state.Nodes[0]
	if node.LatestSourceLabel != "Agent Chat" {
		t.Fatalf("LatestSourceLabel = %q, want Agent Chat", node.LatestSourceLabel)
	}
	wantHref := thoughtsDocRedirectURLForRoot(service.thoughtsRoot, planDir, nil)
	if node.Href != wantHref {
		t.Fatalf("Href = %q, want %q", node.Href, wantHref)
	}
	if node.DirectCount != 2 {
		t.Fatalf(
			"DirectCount = %d, want discovered plus current-user thread",
			node.DirectCount,
		)
	}
}

func TestBuildPlanSidebarStateOverlayHrefSurvivesNewerArtifactTimestamp(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "newer-artifact-plan")
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Newer Artifact Plan",
		time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	workspace, err := service.CreateWorkspace(
		t.Context(),
		WorkspaceCreateInput{
			UserEmail:   "user@example.com",
			Title:       "Current User Workspace",
			RootDocPath: planDir,
			Cwd:         planDir,
			Source:      WorkspaceSourceWeb,
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	thread := mustCreateAgentThread(
		t,
		service,
		"newer-artifact-thread",
		"user@example.com",
		service.projectRoot,
		"newer-artifact-lineage",
	)
	if err := service.SetThreadPrimaryWorkspace(t.Context(), thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("attach thread: %v", err)
	}

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 1 {
		t.Fatalf("nodes = %#v, want one merged plan node", state.Nodes)
	}
	node := state.Nodes[0]
	wantHref := thoughtsDocRedirectURLForRoot(service.thoughtsRoot, planDir, nil)
	if node.Href != wantHref {
		t.Fatalf("Href = %q, want thoughts plan href %q", node.Href, wantHref)
	}
	if node.LatestSourceLabel != "Artifact" {
		t.Fatalf("LatestSourceLabel = %q, want Artifact", node.LatestSourceLabel)
	}
}

func TestBuildPlanSidebarStateDoesNotExposeCrossUserWorkspaceHref(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "private-plan")
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		planDir,
		"Private Plan",
		time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC),
	)
	workspace, err := service.CreateWorkspace(
		t.Context(),
		WorkspaceCreateInput{
			UserEmail:   "other@example.com",
			Title:       "Other Workspace",
			RootDocPath: planDir,
			Cwd:         planDir,
			Source:      WorkspaceSourceWeb,
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	thread := mustCreateAgentThread(
		t,
		service,
		"other-thread",
		"other@example.com",
		service.projectRoot,
		"other-lineage",
	)
	if err := service.SetThreadPrimaryWorkspace(t.Context(), thread.ID, workspace.ID, "test"); err != nil {
		t.Fatalf("attach other thread: %v", err)
	}

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 1 {
		t.Fatalf("nodes = %#v, want discovered-only plan node", state.Nodes)
	}
	if strings.Contains(state.Nodes[0].Href, workspace.ID) {
		t.Fatalf(
			"Href = %q, leaked cross-user workspace %q",
			state.Nodes[0].Href,
			workspace.ID,
		)
	}
	wantHref := thoughtsDocRedirectURLForRoot(service.thoughtsRoot, planDir, nil)
	if state.Nodes[0].Href != wantHref {
		t.Fatalf("Href = %q, want discovered plan href %q", state.Nodes[0].Href, wantHref)
	}
}

func TestBuildPlanSidebarStateSortsDiscoveredByArtifactTimestamp(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	oldPlan := filepath.Join(service.thoughtsRoot, "user", "plans", "old-plan")
	newPlan := filepath.Join(service.thoughtsRoot, "user", "plans", "new-plan")
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		oldPlan,
		"Old Plan",
		time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC),
	)
	mustUpsertDiscoveredPlanWorkspace(
		t,
		service,
		newPlan,
		"New Plan",
		time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC),
	)

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() error = %v", err)
	}
	if len(state.Nodes) != 2 || state.Nodes[0].PlanDir != newPlan ||
		state.Nodes[1].PlanDir != oldPlan {
		t.Fatalf("nodes = %#v, want new plan before old plan", state.Nodes)
	}
}

func TestBuildPlanSidebarStateDoesNotWalkPiSessionsDir(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "hot-path")
	mustCreateAgentSession(t, service, "session-1", "user@example.com", planDir, "", "")
	blockedDir := t.TempDir()
	if err := os.Chmod(blockedDir, 0o000); err != nil {
		t.Fatalf("Chmod(blockedDir) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blockedDir, 0o755) })
	service.piSessionsDir = blockedDir

	state, err := service.BuildPlanSidebarState(
		t.Context(),
		PlanSidebarInput{UserEmail: "user@example.com"},
	)
	if err != nil {
		t.Fatalf("BuildPlanSidebarState() should not scan piSessionsDir: %v", err)
	}
	if len(state.Nodes) != 1 || state.Nodes[0].PlanDir != planDir {
		t.Fatalf("nodes = %#v, want DB-backed plan node", state.Nodes)
	}
}

func TestRemapCopiedWorkspacePlanDirMapsThoughtsPathFromCopiedCheckout(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	copiedPlan := filepath.Join(
		t.TempDir(),
		"cn-agents-2026-copy",
		"thoughts",
		"user",
		"plans",
		"durable-plan",
		"reviews",
		"2026-05-16_implementation-review",
		"plan.md",
	)

	got, ok := service.remapCopiedWorkspacePlanDir(copiedPlan)
	want := filepath.Join(
		service.thoughtsRoot,
		"user",
		"plans",
		"durable-plan",
		"reviews",
		"2026-05-16_implementation-review",
	)
	if !ok || got != want {
		t.Fatalf("remapCopiedWorkspacePlanDir() = %q, %v; want %q, true", got, ok, want)
	}
}

func TestRemapCopiedWorkspacePlanDirRejectsOutOfProjectCandidates(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)

	for _, candidate := range []string{
		"",
		filepath.Join(t.TempDir(), "notes", "user", "plans", "not-thoughts"),
		filepath.Join(t.TempDir(), "thoughts", "user", "scratch", "not-plan"),
	} {
		if got, ok := service.remapCopiedWorkspacePlanDir(candidate); ok || got != "" {
			t.Fatalf(
				"remapCopiedWorkspacePlanDir(%q) = %q, %v; want empty false",
				candidate,
				got,
				ok,
			)
		}
	}
}

func TestIndexPiSessionsUpsertsInferredPlanDir(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "indexed-plan")
	copiedPlanDir := filepath.Join(
		t.TempDir(),
		"cn-agents-copy",
		"thoughts",
		"user",
		"plans",
		"indexed-plan",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "session-indexed.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","id":"session-indexed","timestamp":"2026-05-16T12:00:00Z","cwd":`+strconv.Quote(
			copiedPlanDir,
		)+`}`,
		`{"type":"message","id":"user-1","timestamp":"2026-05-16T12:00:01Z","message":{"role":"user","content":"hello"}}`,
	)

	result, err := service.indexPiSessions(
		t.Context(),
		PiSessionIndexRequest{UserEmail: "User@Example.com", Reason: "test"},
	)
	if err != nil {
		t.Fatalf("indexPiSessions() error = %v", err)
	}
	if result.Indexed != 1 || result.Failed != 0 {
		t.Fatalf("indexPiSessions() = %+v, want one indexed", result)
	}
	session, err := service.queries.GetAgentSessionByPath(
		t.Context(),
		nullString(sessionPath),
	)
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	if session.IndexedByUserEmail.String != "user@example.com" ||
		session.PlanDir.String != planDir ||
		session.ExternalSessionID.String != "session-indexed" {
		t.Fatalf(
			"indexed session = %#v, want normalized user and plan dir %q",
			session,
			planDir,
		)
	}
}

func TestPiSessionFilesSurviveWorkspaceCleanupLocation(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	service.piSessionsDir = t.TempDir()

	workspaceCheckout := filepath.Join(t.TempDir(), "vamos-feature")
	workspacePlanDir := filepath.Join(workspaceCheckout, "thoughts", "user", "plans", "cleanup-proof")
	if err := os.MkdirAll(filepath.Join(workspaceCheckout, ".vamos", "state"), 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace state) error = %v", err)
	}
	if err := os.MkdirAll(workspacePlanDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspace plan) error = %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "workspace", "cleanup-proof.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","id":"cleanup-proof","timestamp":"2026-05-24T12:00:00Z","cwd":`+strconv.Quote(workspacePlanDir)+`}`,
		`{"type":"message","id":"user-1","timestamp":"2026-05-24T12:00:01Z","message":{"role":"user","content":"sandbox verification"}}`,
	)

	if err := os.RemoveAll(workspaceCheckout); err != nil {
		t.Fatalf("RemoveAll(workspaceCheckout) error = %v", err)
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("global Pi session file should survive checkout cleanup: %v", err)
	}

	result, err := service.indexPiSessions(
		t.Context(),
		PiSessionIndexRequest{UserEmail: "user@example.com", Reason: "cleanup-test"},
	)
	if err != nil {
		t.Fatalf("indexPiSessions() error = %v", err)
	}
	if result.Indexed != 1 || result.Failed != 0 {
		t.Fatalf("indexPiSessions() = %+v, want one indexed", result)
	}
	indexed, err := service.queries.GetAgentSessionByPath(t.Context(), nullString(sessionPath))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	wantPlanDir := filepath.Join(service.thoughtsRoot, "user", "plans", "cleanup-proof")
	if indexed.PlanDir.String != wantPlanDir || indexed.ProjectedThreadID.Valid {
		t.Fatalf("indexed session = %#v, want remapped plan %q and no imported thread", indexed, wantPlanDir)
	}
}

func TestPiSessionIndexDoesNotImportTranscriptWithoutOpenAction(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)

	copiedPlanDir := filepath.Join(
		t.TempDir(),
		"vamos-copy",
		"thoughts",
		"user",
		"plans",
		"index-only",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "index-only.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","id":"index-only","timestamp":"2026-05-24T12:00:00Z","cwd":`+strconv.Quote(copiedPlanDir)+`}`,
		`{"type":"message","id":"user-1","parentId":null,"timestamp":"2026-05-24T12:00:01Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"custom","id":"custom-1","parentId":"user-1","timestamp":"2026-05-24T12:00:02Z","customType":"plan-classification","data":{"planDir":"thoughts/user/plans/index-only","source":"prompt-path"}}`,
		`{"type":"message","id":"assistant-1","parentId":"custom-1","timestamp":"2026-05-24T12:00:03Z","message":{"role":"assistant","content":"world"}}`,
	)

	result, err := service.indexPiSessions(
		t.Context(),
		PiSessionIndexRequest{UserEmail: "user@example.com", Reason: "index-only"},
	)
	if err != nil {
		t.Fatalf("indexPiSessions() error = %v", err)
	}
	if result.Indexed != 1 || result.Imported != 0 {
		t.Fatalf("indexPiSessions() = %+v, want metadata index without import", result)
	}
	threads, err := service.queries.ListAgentThreads(
		t.Context(),
		db.ListAgentThreadsParams{UserEmail: "user@example.com", Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListAgentThreads() error = %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("indexed Pi session created %d transcript thread(s), want 0", len(threads))
	}
	indexed, err := service.queries.GetAgentSessionByPath(t.Context(), nullString(sessionPath))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	if indexed.ProjectedThreadID.Valid {
		t.Fatalf("indexed session = %#v, want metadata without thread import", indexed)
	}
}

func TestRequestPiSessionIndexCoalescesPerUser(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	service.piIndexMu.Lock()
	service.piIndexRunning["user@example.com"] = true
	service.piIndexMu.Unlock()

	service.RequestPiSessionIndex(
		PiSessionIndexRequest{UserEmail: "User@Example.com", Reason: "first"},
	)
	service.RequestPiSessionIndex(
		PiSessionIndexRequest{
			UserEmail: "user@example.com",
			Reason:    "second",
			Force:     true,
		},
	)

	service.piIndexMu.Lock()
	defer service.piIndexMu.Unlock()
	queued, ok := service.piIndexQueued["user@example.com"]
	if !ok || !queued.Force || !strings.Contains(queued.Reason, "first") ||
		!strings.Contains(queued.Reason, "second") {
		t.Fatalf("queued = %#v, %v; want merged force request", queued, ok)
	}
}

func TestBuildPageArgsDoesNotRequirePiSessionsDir(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	planDir := filepath.Join(service.thoughtsRoot, "user", "plans", "first-render")
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		planDir,
		"lineage-1",
	)
	blockedDir := t.TempDir()
	if err := os.Chmod(blockedDir, 0o000); err != nil {
		t.Fatalf("Chmod(blockedDir) error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blockedDir, 0o755) })
	service.piSessionsDir = blockedDir

	args, err := service.BuildPageArgs(
		t.Context(),
		"user@example.com",
		thread.ID,
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("BuildPageArgs() should not scan piSessionsDir: %v", err)
	}
	if args.PlanSidebar.TargetID != "agent-chat-thread-sidebar" ||
		len(args.PlanSidebar.Nodes) == 0 {
		t.Fatalf(
			"PlanSidebar = %#v, want initialized DB-backed sidebar",
			args.PlanSidebar,
		)
	}
}

func TestCollectPlanSidebarSourcesUsesTenantSafeWorkspaceJoin(t *testing.T) {
	t.Parallel()
	service := newTestAgentChatService(t)
	projectRoot := t.TempDir()
	root := filepath.Join(projectRoot, "thoughts")
	service.projectRoot = projectRoot
	service.thoughtsRoot = root
	ownerPlan := filepath.Join(root, "owner@example.com", "plans", "owner-plan")
	foreignPlan := filepath.Join(root, "owner@example.com", "plans", "foreign-plan")
	if err := os.MkdirAll(ownerPlan, 0o755); err != nil {
		t.Fatalf("mkdir owner plan: %v", err)
	}
	if err := os.MkdirAll(foreignPlan, 0o755); err != nil {
		t.Fatalf("mkdir foreign plan: %v", err)
	}
	ownerWorkspace, err := service.CreateWorkspace(
		t.Context(),
		WorkspaceCreateInput{
			UserEmail:   "owner@example.com",
			Title:       "Owner Workspace",
			RootDocPath: ownerPlan,
			Cwd:         ownerPlan,
			Source:      WorkspaceSourceWeb,
		},
	)
	if err != nil {
		t.Fatalf("create owner workspace: %v", err)
	}
	foreignWorkspace, err := service.CreateWorkspace(
		t.Context(),
		WorkspaceCreateInput{
			UserEmail:   "other@example.com",
			Title:       "Foreign Workspace",
			RootDocPath: foreignPlan,
			Cwd:         foreignPlan,
			Source:      WorkspaceSourceWeb,
		},
	)
	if err != nil {
		t.Fatalf("create foreign workspace: %v", err)
	}
	ownerThread := mustCreateAgentThread(
		t,
		service,
		"owner-thread",
		"owner@example.com",
		service.projectRoot,
		"owner-lineage",
	)
	if err := service.SetThreadPrimaryWorkspace(t.Context(), ownerThread.ID, ownerWorkspace.ID, "test"); err != nil {
		t.Fatalf("attach owner thread: %v", err)
	}
	foreignThread := mustCreateAgentThread(
		t,
		service,
		"foreign-thread",
		"owner@example.com",
		service.projectRoot,
		"foreign-lineage",
	)
	if err := service.SetThreadPrimaryWorkspace(t.Context(), foreignThread.ID, foreignWorkspace.ID, "test"); err != nil {
		t.Fatalf("attach foreign thread: %v", err)
	}

	sources, err := service.collectPlanSidebarSources(
		t.Context(),
		"owner@example.com",
		"",
	)
	if err != nil {
		t.Fatalf("collectPlanSidebarSources() error = %v", err)
	}
	if len(sources) != 1 || sources[0].PlanDir != ownerPlan {
		t.Fatalf("sources = %#v, want only owned workspace artifact root", sources)
	}
}

func TestBuildStableTranscriptOmitsHeaderMetadataEntries(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"model-1",
		"",
		"model_change",
		0,
		`{"type":"model_change","id":"model-1","parentId":null,"timestamp":"2026-04-19T12:00:00Z","provider":"openai-codex","modelId":"gpt-5.4"}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"think-1",
		"model-1",
		"thinking_level_change",
		1,
		`{"type":"thinking_level_change","id":"think-1","parentId":"model-1","timestamp":"2026-04-19T12:00:01Z","thinkingLevel":"high"}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"user-1",
		"think-1",
		"message",
		2,
		`{"type":"message","id":"user-1","parentId":"think-1","timestamp":"2026-04-19T12:00:02Z","message":{"role":"user","content":"hello"}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-1",
		"user-1",
		"message",
		3,
		`{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-04-19T12:00:03Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Plan first"},{"type":"toolCall","id":"call-1","name":"read","arguments":{"path":"main.go"}}]}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"tool-1",
		"assistant-1",
		"message",
		4,
		`{"type":"message","id":"tool-1","parentId":"assistant-1","timestamp":"2026-04-19T12:00:04Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"read","content":[{"type":"text","text":"}\n\tfoo := 1\n}"}],"details":{},"isError":false}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-2",
		"tool-1",
		"message",
		5,
		`{"type":"message","id":"assistant-2","parentId":"tool-1","timestamp":"2026-04-19T12:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"Done"}]}}`,
	)
	if err := service.queries.UpdateAgentThreadHead(
		context.Background(),
		db.UpdateAgentThreadHeadParams{
			ID:          thread.ID,
			HeadEntryID: sql.NullString{String: "assistant-2", Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}
	thread.HeadEntryID = sql.NullString{String: "assistant-2", Valid: true}

	metadata, err := service.buildTranscriptMetadata(t.Context(), thread)
	if err != nil {
		t.Fatalf("buildTranscriptMetadata() error = %v", err)
	}
	if metadata.ModelLabel != "openai-codex / gpt-5.4" ||
		metadata.ThinkingLabel != "high" {
		t.Fatalf("metadata = %+v, want model and thinking labels", metadata)
	}

	messages, err := service.buildStableTranscript(t.Context(), thread)
	if err != nil {
		t.Fatalf("buildStableTranscript() error = %v", err)
	}

	if len(messages) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "hello" {
		t.Fatalf("messages[0] = %#v, want user bubble", messages[0])
	}
	if messages[1].Title != "thinking" || !messages[1].Collapsible ||
		!messages[1].HideBodyWhenCollapsed {
		t.Fatalf(
			"messages[1] = %#v, want auto-minimized thinking detail",
			messages[1],
		)
	}
	if !strings.Contains(messages[1].HeaderSummary, "[10] Plan first") {
		t.Fatalf(
			"messages[1].HeaderSummary = %q, want thinking preview with character count",
			messages[1].HeaderSummary,
		)
	}
	if !strings.Contains(messages[1].HTMLContent, "<blockquote>") {
		t.Fatalf(
			"messages[1].HTMLContent = %q, want blockquote-rendered thinking",
			messages[1].HTMLContent,
		)
	}
	if messages[2].Title != "read" || !messages[2].Collapsible ||
		!messages[2].HideBodyWhenCollapsed {
		t.Fatalf("messages[2] = %#v, want combined hidden read detail", messages[2])
	}
	if messages[2].HeaderCode != projectMainPath {
		t.Fatalf(
			"messages[2].HeaderCode = %q, want %s",
			messages[2].HeaderCode,
			projectMainPath,
		)
	}
	if !strings.Contains(messages[2].HTMLContent, `class="markdown-code-block"`) {
		t.Fatalf(
			"messages[2].HTMLContent = %q, want syntax-highlighted markdown code block",
			messages[2].HTMLContent,
		)
	}
	if strings.Contains(messages[2].HTMLContent, `<p class="text-lg">`) {
		t.Fatalf(
			"messages[2].HTMLContent = %q, want no stray paragraph-wrapped braces",
			messages[2].HTMLContent,
		)
	}
	if messages[3].Role != "assistant" || messages[3].Content != "Done" {
		t.Fatalf("messages[3] = %#v, want final assistant bubble", messages[3])
	}
}

func TestReadToolResultUsesMarkdownCodeBlockAndCollapseLimit(t *testing.T) {
	service := newTestAgentChatService(t)
	service.detailCollapseLineLimit = 2

	items := service.messageTranscriptItems(
		"tool-1",
		"tool-1",
		"toolResult",
		[]any{map[string]any{"type": "text", "text": "line 1\nline 2\nline 3"}},
		false,
		"read",
		"call-1",
		nil,
		false,
		map[string]TranscriptMessage{"call-1": {HeaderCode: projectMainPath}},
	)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if !items[0].Collapsible || !items[0].HideBodyWhenCollapsed {
		t.Fatalf("items[0] = %#v, want hidden collapsible read output", items[0])
	}
	if items[0].HeaderCode != projectMainPath {
		t.Fatalf(
			"items[0].HeaderCode = %q, want %s",
			items[0].HeaderCode,
			projectMainPath,
		)
	}
	if !strings.Contains(items[0].HTMLContent, `class="markdown-code-block"`) {
		t.Fatalf(
			"items[0].HTMLContent = %q, want markdown code block",
			items[0].HTMLContent,
		)
	}
	if !strings.Contains(items[0].Content, "````") ||
		strings.Contains(items[0].Content, projectMainPath) {
		t.Fatalf(
			"items[0].Content = %q, want fenced output-only markdown code block",
			items[0].Content,
		)
	}
	if items[0].DetailHeader != projectMainPath {
		t.Fatalf("DetailHeader = %q, want full path", items[0].DetailHeader)
	}
}

func TestBashToolResultUsesHiddenCodeBlock(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	service.detailCollapseLineLimit = 1

	items := service.messageTranscriptItems(
		"tool-1",
		"tool-1",
		"toolResult",
		[]any{map[string]any{"type": "text", "text": "line 1\nline 2"}},
		false,
		"bash",
		"call-1",
		nil,
		false,
		map[string]TranscriptMessage{"call-1": {HeaderCode: "go test ./..."}},
	)
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Title != "bash" || items[0].HeaderCode != "go test ./..." {
		t.Fatalf("item = %#v, want bash output paired with command header", items[0])
	}
	if !items[0].Collapsible || !items[0].HideBodyWhenCollapsed {
		t.Fatalf("item = %#v, want hidden collapsible bash output", items[0])
	}
	if !strings.Contains(items[0].HTMLContent, `class="markdown-code-block"`) {
		t.Fatalf("HTMLContent = %q, want markdown code block", items[0].HTMLContent)
	}
	if !strings.Contains(items[0].Content, "````") ||
		strings.Contains(items[0].Content, "$ go test ./...") {
		t.Fatalf("Content = %q, want fenced output-only code block", items[0].Content)
	}
	if items[0].DetailHeader != "$ go test ./..." {
		t.Fatalf("DetailHeader = %q, want full command", items[0].DetailHeader)
	}
}

func TestAssistantTranscriptItemsCompactsBashToolCall(t *testing.T) {
	service := newTestAgentChatService(t)
	items := service.assistantTranscriptItems("msg-1", "entry-1", []any{map[string]any{
		"type":      "toolCall",
		"name":      "bash",
		"arguments": map[string]any{"command": "ls -la", "timeout": 10},
	}}, false, nil)

	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Title != "bash" {
		t.Fatalf("Title = %q, want bash", items[0].Title)
	}
	if items[0].HeaderCode != "ls -la" {
		t.Fatalf("HeaderCode = %q, want command summary", items[0].HeaderCode)
	}
	if !items[0].HideBodyWhenCollapsed || !items[0].Collapsible {
		t.Fatalf("item = %#v, want hidden collapsible body for bash tool call", items[0])
	}
	if !strings.Contains(items[0].Content, `"command": "ls -la"`) {
		t.Fatalf("Content = %q, want full json body", items[0].Content)
	}
}

func TestAssistantTranscriptItemsCompactsReadToolCall(t *testing.T) {
	service := newTestAgentChatService(t)
	items := service.assistantTranscriptItems("msg-1", "entry-1", []any{map[string]any{
		"type":      "toolCall",
		"name":      "read",
		"arguments": map[string]any{"path": "server/services/agentchat/service.go"},
	}}, false, nil)

	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Title != "read" {
		t.Fatalf("Title = %q, want read", items[0].Title)
	}
	if items[0].HeaderCode != "project/server/services/agentchat/service.go" {
		t.Fatalf("HeaderCode = %q, want project-root path summary", items[0].HeaderCode)
	}
	if !items[0].HideBodyWhenCollapsed || !items[0].Collapsible {
		t.Fatalf("item = %#v, want hidden collapsible body for read tool call", items[0])
	}
	if !strings.Contains(
		items[0].Content,
		`"path": "server/services/agentchat/service.go"`,
	) {
		t.Fatalf("Content = %q, want full json body", items[0].Content)
	}
}

func TestAssistantTranscriptItemsCompactsSubagentToolCall(t *testing.T) {
	service := newTestAgentChatService(t)
	items := service.assistantTranscriptItems("msg-1", "entry-1", []any{
		map[string]any{
			"type": "toolCall",
			"name": "subagent",
			"arguments": map[string]any{
				"agent": "scout",
				"task":  "Analyze the repository layout",
			},
		},
	}, false, nil)

	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Title != "subagent" {
		t.Fatalf("Title = %q, want subagent", items[0].Title)
	}
	if !strings.Contains(items[0].HeaderSummary, "agent: scout") ||
		!strings.Contains(items[0].HeaderSummary, "task: Analyze the repository layout") {
		t.Fatalf(
			"HeaderSummary = %q, want compact agent/task summary",
			items[0].HeaderSummary,
		)
	}
	if !items[0].HideBodyWhenCollapsed || !items[0].Collapsible {
		t.Fatalf(
			"item = %#v, want hidden collapsible body for subagent tool call",
			items[0],
		)
	}
	if !strings.Contains(items[0].Content, `"agent": "scout"`) {
		t.Fatalf("Content = %q, want full json body", items[0].Content)
	}
}

func TestNewDetailTranscriptMessageCollapsesOnlyAfterLineLimit(t *testing.T) {
	service := newTestAgentChatService(t)
	service.detailCollapseLineLimit = 2

	short := service.newDetailTranscriptMessage(
		"short",
		"short",
		"thinking",
		"line 1\nline 2",
		false,
		true,
	)
	if short.Collapsible {
		t.Fatalf("short.Collapsible = true, want false")
	}

	long := service.newDetailTranscriptMessage(
		"long",
		"long",
		"thinking",
		"line 1\nline 2\nline 3",
		false,
		true,
	)
	if !long.Collapsible {
		t.Fatalf("long.Collapsible = false, want true")
	}
}

func TestDetailLineCountIgnoresCodeFences(t *testing.T) {
	if got := detailLineCount("```json\n{\n  \"path\": \"main.go\"\n}\n```"); got != 3 {
		t.Fatalf("detailLineCount() = %d, want 3", got)
	}
}

func TestBuildTranscriptSeparatesStableAndLiveItems(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"user-1",
		"",
		"message",
		0,
		`{"type":"message","id":"user-1","parentId":null,"timestamp":"2026-04-19T12:00:00Z","message":{"role":"user","content":"persisted"}}`,
	)
	if err := service.queries.UpdateAgentThreadHead(
		context.Background(),
		db.UpdateAgentThreadHeadParams{
			ID:          thread.ID,
			HeadEntryID: sql.NullString{String: "user-1", Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}
	thread.HeadEntryID = sql.NullString{String: "user-1", Valid: true}

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       "run-1",
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"live"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	stable, err := service.buildStableTranscript(context.Background(), thread)
	if err != nil {
		t.Fatalf("buildStableTranscript() error = %v", err)
	}
	live, _ := service.buildLiveTranscript(thread.ID)

	if len(stable) != 1 || stable[0].Content != "persisted" {
		t.Fatalf("stable = %#v", stable)
	}
	if len(live.Items) != 1 || live.Items[0].Content != "live" {
		t.Fatalf("live = %#v", live.Items)
	}
}

func TestBuildLiveTranscriptIncludesThinkingAndToolCalls(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	events := []conversation.EventEnvelope{
		{
			RunID:       "run-1",
			ThreadID:    thread.ID,
			EventType:   "message_end",
			PayloadJSON: `{"message":{"role":"user","content":"hi"}}`,
		},
		{
			RunID:       "run-1",
			ThreadID:    thread.ID,
			EventType:   "message_update",
			PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"thinking","thinking":"plan"},{"type":"text","text":"answer"},{"type":"toolCall","id":"call-1","name":"read","arguments":{"path":"main.go"}}]}}`,
		},
		{
			RunID:       "run-1",
			ThreadID:    thread.ID,
			EventType:   "tool_execution_update",
			PayloadJSON: `{"toolCallId":"call-1","toolName":"read","args":{"path":"main.go"},"partialResult":{"content":[{"type":"text","text":"partial"}],"details":{}}}`,
		},
	}
	for _, env := range events {
		if err := service.ApplyLiveEvent(env); err != nil {
			t.Fatalf("ApplyLiveEvent(%s) error = %v", env.EventType, err)
		}
	}

	live, _ := service.buildLiveTranscript(thread.ID)
	var hasThinking bool
	var hasAssistantText bool
	var hasToolExecution bool
	for _, item := range live.Items {
		if item.Title == "thinking" {
			hasThinking = true
		}
		if item.Role == "assistant" && item.Content == "answer" {
			hasAssistantText = true
		}
		if item.Title == "read" && strings.Contains(item.HeaderSummary, "running") &&
			strings.Contains(item.Content, "partial") {
			hasToolExecution = true
		}
	}

	if !hasThinking {
		t.Fatalf("live items = %#v, want thinking detail", live.Items)
	}
	if !hasAssistantText {
		t.Fatalf("live items = %#v, want assistant text bubble", live.Items)
	}
	if !hasToolExecution {
		t.Fatalf("live items = %#v, want running tool execution detail", live.Items)
	}
}

func TestCheckpointMovesLiveItemsIntoStableTranscript(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")
	signalCh := service.notifier.Subscribe(thread.ID)
	defer service.notifier.Unsubscribe(thread.ID, signalCh)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"working"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	liveBefore, _ := service.buildLiveTranscript(thread.ID)
	if len(liveBefore.Items) != 1 || liveBefore.Items[0].Content != "working" {
		t.Fatalf("liveBefore = %#v", liveBefore.Items)
	}

	cp := conversation.Checkpoint{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		HeadEntryID: "assistant-1",
		TurnIndex:   1,
		Header:      conversation.SnapshotHeader{SessionID: thread.ID, Cwd: thread.Cwd},
		NewEntries: []conversation.SnapshotEntry{
			{
				LineageID:   thread.LineageID,
				EntryID:     "assistant-1",
				EntryType:   "message",
				Timestamp:   time.Now().UTC(),
				OriginOrder: 0,
				PayloadJSON: `{"type":"message","id":"assistant-1","parentId":null,"timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`,
			},
		},
	}
	if err := service.ApplyCheckpoint(context.Background(), cp); err != nil {
		t.Fatalf("ApplyCheckpoint() error = %v", err)
	}

	gotScopes := readNotifierScopes(t, signalCh, 2)
	wantScopes := []StreamPatchScope{
		PatchStableTranscript,
		PatchArtifactPane,
	}
	if !reflect.DeepEqual(gotScopes, wantScopes) {
		t.Fatalf("checkpoint scopes = %v, want %v", gotScopes, wantScopes)
	}

	updated, err := service.queries.GetAgentThread(context.Background(), thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	stable, err := service.buildStableTranscript(context.Background(), updated)
	if err != nil {
		t.Fatalf("buildStableTranscript() error = %v", err)
	}
	liveAfter, _ := service.buildLiveTranscript(thread.ID)

	if len(stable) != 1 || stable[0].Content != "done" {
		t.Fatalf("stable = %#v", stable)
	}
	if len(liveAfter.Items) != 0 {
		t.Fatalf("liveAfter = %#v, want empty live region", liveAfter.Items)
	}
}

func TestFinalizeRunClearsLiveStateAndNotifiesRunHeader(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")
	signalCh := service.notifier.Subscribe(thread.ID)
	defer service.notifier.Unsubscribe(thread.ID, signalCh)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"partial"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	if err := service.FinalizeRun(
		context.Background(),
		conversation.RunResult{RunID: run.ID, ThreadID: thread.ID},
	); err != nil {
		t.Fatalf("FinalizeRun() error = %v", err)
	}

	gotScopes := readNotifierScopes(t, signalCh, 1)
	wantScopes := []StreamPatchScope{PatchRunHeader}
	if !reflect.DeepEqual(gotScopes, wantScopes) {
		t.Fatalf("finalize scopes = %v, want %v", gotScopes, wantScopes)
	}

	live, _ := service.buildLiveTranscript(thread.ID)
	if len(live.Items) != 0 {
		t.Fatalf("live.Items = %#v, want empty after finalize", live.Items)
	}
}

func TestFailRunClearsLiveStateAndNotifiesRunHeader(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")
	signalCh := service.notifier.Subscribe(thread.ID)
	defer service.notifier.Unsubscribe(thread.ID, signalCh)

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"text","text":"partial"}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	if err := service.FailRun(
		context.Background(),
		conversation.RunFailure{RunID: run.ID, ErrorMessage: "boom"},
	); err != nil {
		t.Fatalf("FailRun() error = %v", err)
	}

	gotScopes := readNotifierScopes(t, signalCh, 1)
	wantScopes := []StreamPatchScope{PatchRunHeader}
	if !reflect.DeepEqual(gotScopes, wantScopes) {
		t.Fatalf("fail scopes = %v, want %v", gotScopes, wantScopes)
	}

	live, _ := service.buildLiveTranscript(thread.ID)
	if len(live.Items) != 0 {
		t.Fatalf("live.Items = %#v, want empty after fail", live.Items)
	}
}

func TestPendingToolCallsDoNotDuplicateAcrossStableAndLiveRegions(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	run := mustCreateAgentRun(t, service, thread.ID, "run-1")

	if err := service.ApplyLiveEvent(conversation.EventEnvelope{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		EventType:   "message_update",
		PayloadJSON: `{"message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"read","arguments":{"path":"main.go"}}]}}`,
	}); err != nil {
		t.Fatalf("ApplyLiveEvent() error = %v", err)
	}

	liveBefore, _ := service.buildLiveTranscript(thread.ID)
	if len(liveBefore.Items) != 1 || liveBefore.Items[0].Title != "read" {
		t.Fatalf("liveBefore = %#v, want pending read tool call", liveBefore.Items)
	}

	cp := conversation.Checkpoint{
		RunID:       run.ID,
		ThreadID:    thread.ID,
		HeadEntryID: "assistant-1",
		TurnIndex:   1,
		Header:      conversation.SnapshotHeader{SessionID: thread.ID, Cwd: thread.Cwd},
		NewEntries: []conversation.SnapshotEntry{
			{
				LineageID:   thread.LineageID,
				EntryID:     "assistant-1",
				EntryType:   "message",
				Timestamp:   time.Now().UTC(),
				OriginOrder: 0,
				PayloadJSON: `{"type":"message","id":"assistant-1","parentId":null,"timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"read","arguments":{"path":"main.go"}}]}}`,
			},
		},
	}
	if err := service.ApplyCheckpoint(context.Background(), cp); err != nil {
		t.Fatalf("ApplyCheckpoint() error = %v", err)
	}

	updated, err := service.queries.GetAgentThread(context.Background(), thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	stable, err := service.buildStableTranscript(context.Background(), updated)
	if err != nil {
		t.Fatalf("buildStableTranscript() error = %v", err)
	}
	liveAfter, _ := service.buildLiveTranscript(thread.ID)

	if len(stable) != 1 || stable[0].Title != "read" {
		t.Fatalf("stable = %#v, want one persisted read tool call", stable)
	}
	if len(liveAfter.Items) != 0 {
		t.Fatalf(
			"liveAfter = %#v, want empty live region after checkpoint",
			liveAfter.Items,
		)
	}
}

func TestBuildSnapshotUsesRequestedHeadPath(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"user-1",
		"",
		"message",
		0,
		`{"type":"message","id":"user-1","parentId":null,"timestamp":"2026-04-19T12:00:00Z","message":{"role":"user","content":"first"}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-1",
		"user-1",
		"message",
		1,
		`{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":"alpha"}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"user-2",
		"assistant-1",
		"message",
		2,
		`{"type":"message","id":"user-2","parentId":"assistant-1","timestamp":"2026-04-19T12:00:02Z","message":{"role":"user","content":"second"}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-2",
		"user-2",
		"message",
		3,
		`{"type":"message","id":"assistant-2","parentId":"user-2","timestamp":"2026-04-19T12:00:03Z","message":{"role":"assistant","content":"bravo"}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		thread.LineageID,
		"assistant-branch",
		"assistant-1",
		"message",
		4,
		`{"type":"message","id":"assistant-branch","parentId":"assistant-1","timestamp":"2026-04-19T12:00:04Z","message":{"role":"assistant","content":"branch"}}`,
	)

	if err := service.queries.UpdateAgentThreadHead(
		context.Background(),
		db.UpdateAgentThreadHeadParams{
			ID:          thread.ID,
			HeadEntryID: sql.NullString{String: "assistant-branch", Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}

	snapshot, err := service.BuildSnapshot(context.Background(), thread.ID, "assistant-2")
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}

	gotIDs := make([]string, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		gotIDs = append(gotIDs, entry.EntryID)
	}

	wantIDs := []string{"user-1", "assistant-1", "user-2", "assistant-2"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("snapshot entry ids = %v, want %v", gotIDs, wantIDs)
	}
	if snapshot.HeadEntryID != "assistant-2" {
		t.Fatalf("HeadEntryID = %q, want assistant-2", snapshot.HeadEntryID)
	}
}

func TestCreateRunRejectsSecondRunningRunForThread(t *testing.T) {
	service := newTestAgentChatService(t)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)

	if _, err := service.createRun(
		context.Background(),
		service.queries,
		thread,
		conversation.RunTriggerSend,
		"first",
		sql.NullString{},
	); err != nil {
		t.Fatalf("first createRun() error = %v", err)
	}

	_, err := service.createRun(
		context.Background(),
		service.queries,
		thread,
		conversation.RunTriggerResume,
		"second",
		sql.NullString{},
	)
	if err == nil {
		t.Fatal("second createRun() error = nil, want active-run conflict")
	}
	if err != ErrThreadRunInProgress {
		t.Fatalf("second createRun() error = %v, want %v", err, ErrThreadRunInProgress)
	}
}

func TestResolveForkRestoreHeadUsesParentForUserMessage(t *testing.T) {
	got := resolveForkRestoreHead("message", "user", "user-2", "assistant-1")
	if got != "assistant-1" {
		t.Fatalf("restore head = %q, want assistant-1", got)
	}
}

func TestResolveForkRestoreHeadKeepsAssistantMessage(t *testing.T) {
	got := resolveForkRestoreHead("message", "assistant", "assistant-2", "user-2")
	if got != "assistant-2" {
		t.Fatalf("restore head = %q, want assistant-2", got)
	}
}

func TestCreateForkThreadSharesLineageAndStoresForkOrigin(t *testing.T) {
	service := newTestAgentChatService(t)
	sourceThread := mustCreateAgentThread(
		t,
		service,
		"thread-1",
		"user@example.com",
		"/tmp/project",
		"lineage-1",
	)
	mustCreateAgentEntry(
		t,
		service,
		sourceThread.LineageID,
		"user-1",
		"",
		"message",
		0,
		`{"type":"message","id":"user-1","parentId":null,"timestamp":"2026-04-19T12:00:00Z","message":{"role":"user","content":"hello"}}`,
	)
	mustCreateAgentEntry(
		t,
		service,
		sourceThread.LineageID,
		"assistant-1",
		"user-1",
		"message",
		1,
		`{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-04-19T12:00:01Z","message":{"role":"assistant","content":"world"}}`,
	)

	tx, err := service.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()

	q := service.queries.WithTx(tx)
	sourceEntry, err := q.GetAgentEntry(
		context.Background(),
		db.GetAgentEntryParams{LineageID: sourceThread.LineageID, EntryID: "assistant-1"},
	)
	if err != nil {
		t.Fatalf("GetAgentEntry() error = %v", err)
	}

	thread, run, err := service.createForkThread(
		context.Background(),
		q,
		sourceThread,
		sourceEntry,
		"new direction",
	)
	if err != nil {
		t.Fatalf("createForkThread() error = %v", err)
	}
	if thread.LineageID != sourceThread.LineageID {
		t.Fatalf("LineageID = %q, want %q", thread.LineageID, sourceThread.LineageID)
	}
	if !thread.ParentThreadID.Valid || thread.ParentThreadID.String != sourceThread.ID {
		t.Fatalf("ParentThreadID = %v, want %q", thread.ParentThreadID, sourceThread.ID)
	}
	if !thread.ForkedFromEntryID.Valid ||
		thread.ForkedFromEntryID.String != "assistant-1" {
		t.Fatalf("ForkedFromEntryID = %v, want assistant-1", thread.ForkedFromEntryID)
	}
	if !thread.HeadEntryID.Valid || thread.HeadEntryID.String != "assistant-1" {
		t.Fatalf("HeadEntryID = %v, want assistant-1", thread.HeadEntryID)
	}
	if run.Trigger != string(conversation.RunTriggerFork) {
		t.Fatalf("run trigger = %q, want %q", run.Trigger, conversation.RunTriggerFork)
	}
	if !run.RestoreHeadEntryID.Valid || run.RestoreHeadEntryID.String != "assistant-1" {
		t.Fatalf("RestoreHeadEntryID = %v, want assistant-1", run.RestoreHeadEntryID)
	}
}

func TestBuildArtifactPaneRejectsTraversal(t *testing.T) {
	service := newTestAgentChatService(t)

	_, err := service.renderArtifact("/tmp/project", "../secrets.md")
	if err == nil {
		t.Fatal("renderArtifact() error = nil, want traversal rejection")
	}
}

func TestBuildArtifactPaneListsRenderableFiles(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "plan.md"), "# Plan")
	mustWriteFile(t, filepath.Join(root, "notes.txt"), "hello")
	mustWriteFile(t, filepath.Join(root, "nested", "artifact.yaml"), "key: value")
	mustWriteFile(t, filepath.Join(root, "ignore.png"), "png")
	mustWriteFile(t, filepath.Join(root, ".git", "skip.md"), "# skip")
	mustWriteFile(t, filepath.Join(root, ".pi", "skip.md"), "# skip")
	mustWriteFile(t, filepath.Join(root, "node_modules", "skip.md"), "# skip")

	files, err := listRenderableArtifacts(root)
	if err != nil {
		t.Fatalf("listRenderableArtifacts() error = %v", err)
	}

	want := []string{"nested/artifact.yaml", "notes.txt", "plan.md"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}

	service.projectRoot = root
	service.projectName = filepath.Base(root)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-2",
		"user@example.com",
		root,
		"lineage-2",
	)
	_ = mustCreateAgentRunWithRoot(t, service, "run-2", thread.ID, root)

	pane, err := service.BuildArtifactPane(context.Background(), thread.ID, "run-2", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if pane.ActiveRunID != "run-2" {
		t.Fatalf("ActiveRunID = %q, want run-2", pane.ActiveRunID)
	}
	if pane.WorkingDir.ProjectName == "" || pane.WorkingDir.RelativePath != "" {
		t.Fatalf("WorkingDir = %#v, want project root state", pane.WorkingDir)
	}
	if len(pane.Tree) == 0 || !pane.Tree[0].IsDir || !pane.Tree[0].IsExpanded {
		t.Fatalf(
			"Tree = %#v, want first directory expanded for selected artifact",
			pane.Tree,
		)
	}
	if !pane.Selected.Exists || pane.Selected.RelativePath != "nested/artifact.yaml" {
		t.Fatalf(
			"Selected = %#v, want default selection for nested/artifact.yaml",
			pane.Selected,
		)
	}
}

func TestBuildArtifactPanePrefersDesignDocumentByDefault(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "design.md"), "# Design")
	mustWriteFile(
		t,
		filepath.Join(root, "research", "2026-04-19_research.md"),
		"# Research",
	)
	mustWriteFile(
		t,
		filepath.Join(root, "questions", "2026-04-19_questions.md"),
		"# Questions",
	)

	service.projectRoot = root
	service.projectName = filepath.Base(root)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-4",
		"user@example.com",
		root,
		"lineage-4",
	)
	_ = mustCreateAgentRunWithRoot(t, service, "run-4", thread.ID, root)

	pane, err := service.BuildArtifactPane(context.Background(), thread.ID, "run-4", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if pane.Selected.RelativePath != "design.md" {
		t.Fatalf("Selected.RelativePath = %q, want design.md", pane.Selected.RelativePath)
	}
}

func TestBuildArtifactPanePrefersResearchThenQuestionsByDefault(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()

	mustWriteFile(
		t,
		filepath.Join(root, "research", "2026-04-19_research.md"),
		"# Research",
	)
	mustWriteFile(
		t,
		filepath.Join(root, "questions", "2026-04-19_questions.md"),
		"# Questions",
	)
	mustWriteFile(t, filepath.Join(root, "notes.md"), "# Notes")

	service.projectRoot = root
	service.projectName = filepath.Base(root)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-5",
		"user@example.com",
		root,
		"lineage-5",
	)
	_ = mustCreateAgentRunWithRoot(t, service, "run-5", thread.ID, root)

	pane, err := service.BuildArtifactPane(context.Background(), thread.ID, "run-5", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if pane.Selected.RelativePath != "research/2026-04-19_research.md" {
		t.Fatalf(
			"Selected.RelativePath = %q, want first research doc",
			pane.Selected.RelativePath,
		)
	}

	os.Remove(filepath.Join(root, "research", "2026-04-19_research.md"))
	pane, err = service.BuildArtifactPane(context.Background(), thread.ID, "run-5", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() after removing research error = %v", err)
	}
	if pane.Selected.RelativePath != "questions/2026-04-19_questions.md" {
		t.Fatalf(
			"Selected.RelativePath = %q, want first questions doc",
			pane.Selected.RelativePath,
		)
	}
}

func TestBuildArtifactPaneIgnoresContextResearchWhenChoosingDefault(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()

	mustWriteFile(
		t,
		filepath.Join(root, "context", "research", "2026-04-19_context.md"),
		"# Context Research",
	)
	mustWriteFile(
		t,
		filepath.Join(root, "questions", "2026-04-19_questions.md"),
		"# Questions",
	)
	mustWriteFile(t, filepath.Join(root, "notes.md"), "# Notes")

	service.projectRoot = root
	service.projectName = filepath.Base(root)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-5b",
		"user@example.com",
		root,
		"lineage-5b",
	)
	_ = mustCreateAgentRunWithRoot(t, service, "run-5b", thread.ID, root)

	pane, err := service.BuildArtifactPane(context.Background(), thread.ID, "run-5b", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if pane.Selected.RelativePath != "questions/2026-04-19_questions.md" {
		t.Fatalf(
			"Selected.RelativePath = %q, want top-level questions doc before context research",
			pane.Selected.RelativePath,
		)
	}
}

func TestBuildArtifactPaneSortsTimestampedPlansNewestFirst(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()

	mustWriteFile(
		t,
		filepath.Join(root, "2026-04-19_09-00-00_newer-plan", "plan.md"),
		"# Newer",
	)
	mustWriteFile(
		t,
		filepath.Join(root, "2026-04-18_09-00-00_older-plan", "plan.md"),
		"# Older",
	)
	mustWriteFile(t, filepath.Join(root, "notes.md"), "# Notes")

	files, err := listRenderableArtifacts(root)
	if err != nil {
		t.Fatalf("listRenderableArtifacts() error = %v", err)
	}

	want := []string{
		"2026-04-19_09-00-00_newer-plan/plan.md",
		"2026-04-18_09-00-00_older-plan/plan.md",
		"notes.md",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}

	service.projectRoot = root
	service.projectName = filepath.Base(root)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-6",
		"user@example.com",
		root,
		"lineage-6",
	)
	_ = mustCreateAgentRunWithRoot(t, service, "run-6", thread.ID, root)

	pane, err := service.BuildArtifactPane(context.Background(), thread.ID, "run-6", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if len(pane.Tree) < 2 {
		t.Fatalf("Tree = %#v, want newest and older plan directories", pane.Tree)
	}
	if pane.Tree[0].Name != "2026-04-19_09-00-00_newer-plan" {
		t.Fatalf("pane.Tree[0].Name = %q, want newest timestamp first", pane.Tree[0].Name)
	}
	if pane.Selected.RelativePath != "2026-04-19_09-00-00_newer-plan/plan.md" {
		t.Fatalf(
			"Selected.RelativePath = %q, want newest plan first",
			pane.Selected.RelativePath,
		)
	}
}

func TestBuildFreeformDocsPaneDefaultsToMatchedUserPlansWithoutSelectingDocument(
	t *testing.T,
) {
	t.Parallel()

	service := newTestAgentChatService(t)
	root := t.TempDir()
	service.thoughtsRoot = root
	service.projectRoot = root
	service.projectName = filepath.Base(root)

	mustWriteFile(
		t,
		filepath.Join(root, "CoreyCole", "plans", "2026-05-01_plan", "plan.md"),
		"# Plan",
	)
	mustWriteFile(
		t,
		filepath.Join(root, "OtherUser", "plans", "2026-05-02_other", "plan.md"),
		"# Other",
	)

	pane, err := service.BuildFreeformDocsPane(
		t.Context(),
		"ccoreycole@gmail.com",
		"",
	)
	if err != nil {
		t.Fatalf("BuildFreeformDocsPane() error = %v", err)
	}
	wantRoot := filepath.Join(root, "CoreyCole", "plans")
	if pane.RootDocPath != wantRoot {
		t.Fatalf("RootDocPath = %q, want %q", pane.RootDocPath, wantRoot)
	}
	if len(pane.Tree) == 0 || pane.Tree[0].Name != "2026-05-01_plan" {
		t.Fatalf("Tree = %#v, want current user's plans file tree", pane.Tree)
	}
	if pane.Selected.Exists || pane.Selected.RelativePath != "" {
		t.Fatalf("Selected = %#v, want no default selected document", pane.Selected)
	}
}

func TestBuildFreeformDocsPaneCanSelectDocumentWithoutThread(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	root := t.TempDir()
	service.thoughtsRoot = root
	service.projectRoot = root
	service.projectName = filepath.Base(root)

	mustWriteFile(
		t,
		filepath.Join(root, "CoreyCole", "plans", "2026-05-01_plan", "design.md"),
		"# Design",
	)

	pane, err := service.BuildFreeformDocsPane(
		t.Context(),
		"ccoreycole@gmail.com",
		"2026-05-01_plan/design.md",
	)
	if err != nil {
		t.Fatalf("BuildFreeformDocsPane() error = %v", err)
	}
	if !pane.Selected.Exists ||
		pane.Selected.RelativePath != "2026-05-01_plan/design.md" {
		t.Fatalf("Selected = %#v, want selected design doc", pane.Selected)
	}
}

func TestBuildArtifactPaneFocusesTreeOnSelectedPlanDirectory(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	planRoot := filepath.Join(
		root,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
	)

	mustWriteFile(t, filepath.Join(planRoot, "design.md"), "# Design")
	mustWriteFile(t, filepath.Join(planRoot, "outline.md"), "# Outline")
	mustWriteFile(t, filepath.Join(root, "notes.md"), "# Notes")

	service.projectRoot = root
	service.projectName = filepath.Base(root)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-6b",
		"user@example.com",
		root,
		"lineage-6b",
	)
	_ = mustCreateAgentRunWithRoot(t, service, "run-6b", thread.ID, root)

	pane, err := service.BuildArtifactPane(context.Background(), thread.ID, "run-6b", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if pane.RootDocPath != planRoot {
		t.Fatalf("RootDocPath = %q, want %q", pane.RootDocPath, planRoot)
	}
	if pane.WorkingDir.AbsolutePath != root {
		t.Fatalf(
			"WorkingDir.AbsolutePath = %q, want current cwd %q",
			pane.WorkingDir.AbsolutePath,
			root,
		)
	}
	if pane.WorkingDir.ResetPath != planRoot {
		t.Fatalf(
			"WorkingDir.ResetPath = %q, want %q",
			pane.WorkingDir.ResetPath,
			planRoot,
		)
	}
	if pane.Selected.RelativePath != "design.md" {
		t.Fatalf("Selected.RelativePath = %q, want design.md", pane.Selected.RelativePath)
	}
	if len(pane.Tree) == 0 || pane.Tree[0].Name == "thoughts" {
		t.Fatalf(
			"Tree = %#v, want plan-local files instead of project-root tree",
			pane.Tree,
		)
	}

	pane, err = service.BuildArtifactPane(
		context.Background(),
		thread.ID,
		"run-6b",
		filepath.Join(planRoot, "outline.md"),
	)
	if err != nil {
		t.Fatalf("BuildArtifactPane() with absolute artifact path error = %v", err)
	}
	if pane.Selected.RelativePath != "outline.md" {
		t.Fatalf(
			"Selected.RelativePath = %q, want outline.md",
			pane.Selected.RelativePath,
		)
	}
}

func TestBuildArtifactPaneUsesNestedCwdAsTreeRoot(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	planRoot := filepath.Join(
		root,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
	)
	researchRoot := filepath.Join(planRoot, "research")

	mustWriteFile(t, filepath.Join(planRoot, "design.md"), "# Design")
	mustWriteFile(t, filepath.Join(researchRoot, "2026-04-19_research.md"), "# Research")
	mustWriteFile(t, filepath.Join(researchRoot, "notes.md"), "# Notes")

	service.projectRoot = root
	service.projectName = filepath.Base(root)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-6c",
		"user@example.com",
		researchRoot,
		"lineage-6c",
	)
	_ = mustCreateAgentRunWithRoot(t, service, "run-6c", thread.ID, researchRoot)

	pane, err := service.BuildArtifactPane(context.Background(), thread.ID, "run-6c", "")
	if err != nil {
		t.Fatalf("BuildArtifactPane() error = %v", err)
	}
	if pane.RootDocPath != researchRoot {
		t.Fatalf("RootDocPath = %q, want %q", pane.RootDocPath, researchRoot)
	}
	if pane.WorkingDir.ResetPath != planRoot {
		t.Fatalf(
			"WorkingDir.ResetPath = %q, want %q",
			pane.WorkingDir.ResetPath,
			planRoot,
		)
	}
	if pane.Selected.RelativePath != "2026-04-19_research.md" {
		t.Fatalf(
			"Selected.RelativePath = %q, want research file relative to nested cwd",
			pane.Selected.RelativePath,
		)
	}
	if len(pane.Tree) != 2 {
		t.Fatalf("len(Tree) = %d, want 2 files from nested cwd", len(pane.Tree))
	}
	if pane.Tree[0].Name == "design" || pane.Tree[1].Name == "design" {
		t.Fatalf(
			"Tree = %#v, did not expect sibling plan files while cwd is nested",
			pane.Tree,
		)
	}
}

func TestBuildWorkingDirectoryStateOmitsCurrentDirectoryFromBreadcrumbs(t *testing.T) {
	service := newTestAgentChatService(t)
	projectRoot := t.TempDir()
	service.projectRoot = projectRoot
	service.projectName = filepath.Base(projectRoot)

	cwd := filepath.Join(
		projectRoot,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
		"research",
	)
	state := service.buildWorkingDirectoryState(
		cwd,
		filepath.Join(
			projectRoot,
			"thoughts",
			"creative-mode-agent",
			"plans",
			"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
		),
	)

	if state.CurrentTitle != "research" {
		t.Fatalf("CurrentTitle = %q, want research", state.CurrentTitle)
	}
	if state.CurrentTimestamp != "" {
		t.Fatalf("CurrentTimestamp = %q, want empty", state.CurrentTimestamp)
	}
	if len(state.Crumbs) != 5 {
		t.Fatalf("len(Crumbs) = %d, want 5", len(state.Crumbs))
	}
	if state.Crumbs[len(state.Crumbs)-1].Label != "2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat" {
		t.Fatalf(
			"last crumb = %q, want containing plan directory",
			state.Crumbs[len(state.Crumbs)-1].Label,
		)
	}
}

func TestBuildWorkingDirectoryStateFormatsTimestampedDirectoryTitles(t *testing.T) {
	service := newTestAgentChatService(t)
	projectRoot := t.TempDir()
	service.projectRoot = projectRoot
	service.projectName = filepath.Base(projectRoot)

	cwd := filepath.Join(
		projectRoot,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
	)
	state := service.buildWorkingDirectoryState(cwd, cwd)

	if state.CurrentTitle != "pkg agents sdk pi temporal datastar chat" {
		t.Fatalf("CurrentTitle = %q, want humanized slug", state.CurrentTitle)
	}
	if state.CurrentTimestamp != "2026-04-19 01-47-47" {
		t.Fatalf(
			"CurrentTimestamp = %q, want formatted timestamp",
			state.CurrentTimestamp,
		)
	}
}

func TestFormatSidebarGroupDisplayHumanizesTimestampedPlans(t *testing.T) {
	t.Parallel()

	label, timestamp := formatSidebarGroupDisplay(
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
	)
	if label != "pkg agents sdk pi temporal datastar chat" {
		t.Fatalf("label = %q, want humanized title", label)
	}
	if timestamp != "2026-04-19 01-47-47" {
		t.Fatalf("timestamp = %q, want formatted timestamp", timestamp)
	}
}

func TestUpdateThreadCwdUpdatesStoredDirectory(t *testing.T) {
	service := newTestAgentChatService(t)
	projectRoot := t.TempDir()
	service.projectRoot = projectRoot
	service.projectName = filepath.Base(projectRoot)
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-3",
		"user@example.com",
		projectRoot,
		"lineage-3",
	)
	childDir := filepath.Join(projectRoot, "thoughts", "creative-mode-agent", "plans")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	updated, err := service.UpdateThreadCwd(
		context.Background(),
		"user@example.com",
		thread.ID,
		childDir,
	)
	if err != nil {
		t.Fatalf("UpdateThreadCwd() error = %v", err)
	}
	if updated.Cwd != childDir {
		t.Fatalf("updated cwd = %q, want %q", updated.Cwd, childDir)
	}

	stored, err := service.queries.GetAgentThread(context.Background(), thread.ID)
	if err != nil {
		t.Fatalf("GetAgentThread() error = %v", err)
	}
	if stored.Cwd != childDir {
		t.Fatalf("stored cwd = %q, want %q", stored.Cwd, childDir)
	}
}

func TestNewWorkspaceIDUsesSortableUUIDv7(t *testing.T) {
	first, err := NewWorkspaceID()
	if err != nil {
		t.Fatalf("NewWorkspaceID() first error = %v", err)
	}
	second, err := NewWorkspaceID()
	if err != nil {
		t.Fatalf("NewWorkspaceID() second error = %v", err)
	}
	if first == "" || second == "" {
		t.Fatalf("workspace ids = %q, %q; want non-empty", first, second)
	}
	if first >= second {
		t.Fatalf(
			"workspace ids not sortable by creation order: first=%q second=%q",
			first,
			second,
		)
	}
}

func TestValidateWorkspaceRootDocPathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	userRoot := filepath.Join(thoughtsRoot, "creative-mode-agent")
	planRoot := filepath.Join(userRoot, "plans", "2026-04-30_workspace")
	if err := os.MkdirAll(planRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(planRoot): %v", err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("MkdirAll(outside): %v", err)
	}
	linkPath := filepath.Join(userRoot, "plans", "escape")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := ValidateWorkspaceRootDocPath(
		planRoot,
		thoughtsRoot,
		"creative-mode-agent",
	)
	if err != nil {
		t.Fatalf("ValidateWorkspaceRootDocPath(valid) error = %v", err)
	}
	if got != planRoot {
		t.Fatalf("valid root = %q, want %q", got, planRoot)
	}
	if _, err := ValidateWorkspaceRootDocPath(
		outside,
		thoughtsRoot,
		"creative-mode-agent",
	); err == nil {
		t.Fatal("ValidateWorkspaceRootDocPath(outside) error = nil, want rejection")
	}
	if _, err := ValidateWorkspaceRootDocPath(
		linkPath,
		thoughtsRoot,
		"creative-mode-agent",
	); err == nil {
		t.Fatal(
			"ValidateWorkspaceRootDocPath(symlink escape) error = nil, want rejection",
		)
	}
}

func TestValidateWorkspaceRelPathRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "design.md"),
		[]byte("# Design"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(design): %v", err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(outside, "secret.md"),
		[]byte("secret"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(secret): %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	got, err := ValidateWorkspaceRelPath(root, "./design.md")
	if err != nil {
		t.Fatalf("ValidateWorkspaceRelPath(valid) error = %v", err)
	}
	if got != "design.md" {
		t.Fatalf("valid rel = %q, want design.md", got)
	}
	for _, relPath := range []string{"../secret.md", filepath.Join("escape", "secret.md")} {
		if _, err := ValidateWorkspaceRelPath(root, relPath); err == nil {
			t.Fatalf("ValidateWorkspaceRelPath(%q) error = nil, want rejection", relPath)
		}
	}
}

func TestCreateWorkspaceAndAppendWorkspaceEventIdempotently(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(
		thoughtsRoot,
		"creative-mode-agent",
		"plans",
		"2026-04-30_workspace",
	)
	if err := os.MkdirAll(planRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(planRoot): %v", err)
	}
	service.thoughtsRoot = thoughtsRoot

	workspace, err := service.CreateWorkspace(context.Background(), WorkspaceCreateInput{
		UserEmail:   "creative-mode-agent",
		Title:       "  Workspace title  ",
		RootDocPath: planRoot,
		Cwd:         planRoot,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if workspace.UserEmail != "creative-mode-agent" ||
		workspace.RootDocPath != planRoot ||
		workspace.WorkflowType != string(WorkspaceWorkflowFreeform) ||
		workspace.Source != string(WorkspaceSourceWeb) {
		t.Fatalf("workspace = %#v, want persisted defaults and root", workspace)
	}

	first, err := service.AppendWorkspaceEvent(
		context.Background(),
		service.queries,
		AppendWorkspaceEventInput{
			WorkspaceID: workspace.ID,
			EventType:   "thread_attached",
			ActorType:   "system",
			EventKey:    "thread_attached:thread-1",
		},
	)
	if err != nil {
		t.Fatalf("AppendWorkspaceEvent(first) error = %v", err)
	}
	second, err := service.AppendWorkspaceEvent(
		context.Background(),
		service.queries,
		AppendWorkspaceEventInput{
			WorkspaceID: workspace.ID,
			EventType:   "thread_attached",
			ActorType:   "system",
			EventKey:    "thread_attached:thread-1",
		},
	)
	if err != nil {
		t.Fatalf("AppendWorkspaceEvent(second) error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf(
			"duplicate event ids = %d and %d, want idempotent same row",
			first.ID,
			second.ID,
		)
	}
}

func TestEnsureThreadWorkspaceLazyAttachment(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(
		thoughtsRoot,
		"creative-mode-agent",
		"plans",
		"2026-04-30_workspace",
	)
	if err := os.MkdirAll(planRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(planRoot): %v", err)
	}
	service.thoughtsRoot = thoughtsRoot
	thread := mustCreateAgentThread(
		t,
		service,
		"thread-workspace",
		"creative-mode-agent",
		planRoot,
		"lineage-workspace",
	)

	workspace, attached, err := service.EnsureThreadWorkspace(
		context.Background(),
		"creative-mode-agent",
		thread.ID,
	)
	if err != nil {
		t.Fatalf("EnsureThreadWorkspace() error = %v", err)
	}
	primary, ok, err := service.ResolvePrimaryWorkspaceForThread(context.Background(), "creative-mode-agent", attached.ID)
	if err != nil || !ok || primary.ID != workspace.ID {
		t.Fatalf(
			"primary workspace = (%v, %v, %v), want %s",
			primary.ID,
			ok,
			err,
			workspace.ID,
		)
	}
	if !workspace.SelectedThreadID.Valid ||
		workspace.SelectedThreadID.String != thread.ID {
		t.Fatalf(
			"workspace.SelectedThreadID = %v, want %s",
			workspace.SelectedThreadID,
			thread.ID,
		)
	}
	primaryRow, err := service.queries.GetPrimaryWorkspaceForThread(context.Background(), db.GetPrimaryWorkspaceForThreadParams{ThreadID: thread.ID, UserEmail: "creative-mode-agent"})
	if err != nil {
		t.Fatalf("GetPrimaryWorkspaceForThread() error = %v", err)
	}
	if primaryRow.ID != workspace.ID {
		t.Fatalf("primary.ID = %q, want %s", primaryRow.ID, workspace.ID)
	}
	events, err := service.queries.ListWorkspaceEvents(
		context.Background(),
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].EventType != "thread_attached" {
		t.Fatalf("events = %#v, want one thread_attached event", events)
	}
}

func TestSyncWorkspaceDocInventoryDetectsCreateUpdateDelete(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	artifactRoot := filepath.Join(
		thoughtsRoot,
		"creative-mode-agent",
		"plans",
		"2026-04-30_workspace",
	)
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(artifactRoot): %v", err)
	}
	service.thoughtsRoot = thoughtsRoot
	workspace, err := service.CreateWorkspace(context.Background(), WorkspaceCreateInput{
		UserEmail:   "creative-mode-agent",
		Title:       "Workspace",
		RootDocPath: artifactRoot,
		Cwd:         artifactRoot,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	planPath := filepath.Join(artifactRoot, "plan.md")
	mustWriteFile(t, planPath, "# Plan\n\nFirst")
	changes, err := service.SyncWorkspaceDocInventory(
		context.Background(),
		workspace,
	)
	if err != nil {
		t.Fatalf("SyncWorkspaceDocInventory(create) error = %v", err)
	}
	if len(changes) != 1 || changes[0].RelPath != "plan.md" ||
		changes[0].Action != "created" {
		t.Fatalf("create changes = %#v, want plan.md created", changes)
	}

	changes, err = service.SyncWorkspaceDocInventory(context.Background(), workspace)
	if err != nil {
		t.Fatalf("SyncWorkspaceDocInventory(no-op) error = %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("no-op changes = %#v, want none", changes)
	}

	mustWriteFile(t, planPath, "# Plan\n\nSecond")
	changes, err = service.SyncWorkspaceDocInventory(context.Background(), workspace)
	if err != nil {
		t.Fatalf("SyncWorkspaceDocInventory(update) error = %v", err)
	}
	if len(changes) != 1 || changes[0].RelPath != "plan.md" ||
		changes[0].Action != "updated" {
		t.Fatalf("update changes = %#v, want plan.md updated", changes)
	}

	if err := os.Remove(planPath); err != nil {
		t.Fatalf("Remove(plan.md): %v", err)
	}
	changes, err = service.SyncWorkspaceDocInventory(context.Background(), workspace)
	if err != nil {
		t.Fatalf("SyncWorkspaceDocInventory(delete) error = %v", err)
	}
	if len(changes) != 1 || changes[0].RelPath != "plan.md" ||
		changes[0].Action != "deleted" {
		t.Fatalf("delete changes = %#v, want plan.md deleted", changes)
	}

	events, err := service.queries.ListWorkspaceEvents(
		context.Background(),
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	gotTypes := make([]string, 0, len(events))
	for _, event := range events {
		gotTypes = append(gotTypes, event.EventType)
	}
	wantTypes := []string{"artifact_created", "artifact_updated", "artifact_deleted"}
	if !reflect.DeepEqual(gotTypes, wantTypes) {
		t.Fatalf("event types = %#v, want %#v", gotTypes, wantTypes)
	}
	if _, err := service.queries.GetWorkspaceDoc(
		context.Background(),
		db.GetWorkspaceDocParams{
			WorkspaceID: workspace.ID,
			DocPath:     "thoughts/creative-mode-agent/plans/2026-04-30_workspace/plan.md",
		},
	); err != nil {
		t.Fatalf("GetWorkspaceDoc(deleted row) error = %v", err)
	}
}

func TestBuildWorkspaceDocPaneStoresSelectedRelativePath(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	artifactRoot := filepath.Join(
		thoughtsRoot,
		"creative-mode-agent",
		"plans",
		"2026-04-30_workspace",
	)
	if err := os.MkdirAll(filepath.Join(artifactRoot, "research"), 0o755); err != nil {
		t.Fatalf("MkdirAll(research): %v", err)
	}
	mustWriteFile(t, filepath.Join(artifactRoot, "design.md"), "# Design")
	mustWriteFile(t, filepath.Join(artifactRoot, "research", "notes.md"), "# Notes")
	service.thoughtsRoot = thoughtsRoot
	workspace, err := service.CreateWorkspace(context.Background(), WorkspaceCreateInput{
		UserEmail:   "creative-mode-agent",
		Title:       "Workspace",
		RootDocPath: artifactRoot,
		Cwd:         artifactRoot,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	pane, err := service.BuildWorkspaceDocPane(
		context.Background(),
		workspace,
		"run-1",
		"research/notes.md",
	)
	if err != nil {
		t.Fatalf("BuildWorkspaceDocPane() error = %v", err)
	}
	if pane.Selected.RelativePath != "research/notes.md" {
		t.Fatalf("selected rel = %q, want research/notes.md", pane.Selected.RelativePath)
	}
	stored, err := service.queries.GetWorkspaceForUser(
		context.Background(),
		db.GetWorkspaceForUserParams{ID: workspace.ID, UserEmail: "creative-mode-agent"},
	)
	if err != nil {
		t.Fatalf("GetWorkspaceForUser() error = %v", err)
	}
	if !stored.SelectedDocPath.Valid ||
		stored.SelectedDocPath.String != "research/notes.md" {
		t.Fatalf(
			"selected artifact in DB = %v, want research/notes.md",
			stored.SelectedDocPath,
		)
	}
}

func TestBuildThreadSidebarGroupsUsesPlanDirectoryAncestor(t *testing.T) {
	service := newTestAgentChatService(t)
	projectRoot := t.TempDir()
	service.projectRoot = projectRoot
	service.projectName = filepath.Base(projectRoot)
	service.piSessionsDir = t.TempDir()

	planRoot := filepath.Join(
		projectRoot,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
	)
	refs, err := service.buildSessionSidebarRefs([]db.AgentThread{
		{
			ID:        "thread-1",
			Title:     "Plan root thread",
			Cwd:       planRoot,
			UpdatedAt: time.Now().Add(-2 * time.Minute),
		},
		{
			ID:        "thread-2",
			Title:     "Research thread",
			Cwd:       filepath.Join(planRoot, "research"),
			UpdatedAt: time.Now().Add(-1 * time.Minute),
		},
		{
			ID:        "thread-3",
			Title:     "Workspace thread",
			Cwd:       projectRoot,
			UpdatedAt: time.Now().Add(-3 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("buildSessionSidebarRefs() error = %v", err)
	}

	groups := service.buildThreadSidebarGroups(refs, "thread-2")
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}

	planGroup := groups[0]
	if planGroup.Key != "plan:"+planRoot {
		t.Fatalf("planGroup.Key = %q, want plan root key", planGroup.Key)
	}
	if planGroup.KindLabel != "Plan" {
		t.Fatalf("planGroup.KindLabel = %q, want Plan", planGroup.KindLabel)
	}
	if planGroup.Label != "pkg agents sdk pi temporal datastar chat" {
		t.Fatalf("planGroup.Label = %q, want humanized title", planGroup.Label)
	}
	if planGroup.Timestamp != "2026-04-19 01-47-47" {
		t.Fatalf(
			"planGroup.Timestamp = %q, want formatted timestamp",
			planGroup.Timestamp,
		)
	}
	if planGroup.ThreadCount != 2 {
		t.Fatalf("planGroup.ThreadCount = %d, want 2", planGroup.ThreadCount)
	}
	if !planGroup.IsActive {
		t.Fatalf("planGroup.IsActive = false, want true for active thread group")
	}
	if len(planGroup.Threads) != 2 {
		t.Fatalf("len(planGroup.Threads) = %d, want 2", len(planGroup.Threads))
	}
	if planGroup.ThreadCount != len(planGroup.Threads) {
		t.Fatalf(
			"ThreadCount = %d, len Threads = %d",
			planGroup.ThreadCount,
			len(planGroup.Threads),
		)
	}
	hasActiveThread := false
	for _, thread := range planGroup.Threads {
		if thread.ID == "thread-2" && thread.IsActive {
			hasActiveThread = true
		}
	}
	if planGroup.IsActive != hasActiveThread {
		t.Fatalf("group active state did not match active thread")
	}
	if planGroup.Threads[0].Title != "Research thread" ||
		planGroup.Threads[0].CwdLabel != "research" ||
		!planGroup.Threads[0].IsActive {
		t.Fatalf(
			"planGroup.Threads[0] = %#v, want active research thread",
			planGroup.Threads[0],
		)
	}
	if planGroup.Threads[1].Title != "Plan root thread" ||
		planGroup.Threads[1].CwdLabel != "" {
		t.Fatalf(
			"planGroup.Threads[1] = %#v, want plan-root thread without cwd label",
			planGroup.Threads[1],
		)
	}

	workspaceGroup := groups[1]
	if workspaceGroup.KindLabel != "Workspace" {
		t.Fatalf(
			"workspaceGroup.KindLabel = %q, want Workspace",
			workspaceGroup.KindLabel,
		)
	}
	if workspaceGroup.Label != filepath.Base(projectRoot) {
		t.Fatalf(
			"workspaceGroup.Label = %q, want %q",
			workspaceGroup.Label,
			filepath.Base(projectRoot),
		)
	}
	if len(workspaceGroup.Threads) != 1 || workspaceGroup.Threads[0].CwdLabel != "" {
		t.Fatalf(
			"workspaceGroup.Threads = %#v, want workspace root thread without cwd label",
			workspaceGroup.Threads,
		)
	}
}

func TestBuildSessionSidebarGroupsIncludePlanScopedPiSessions(t *testing.T) {
	service := newTestAgentChatService(t)
	projectRoot := t.TempDir()
	service.projectRoot = projectRoot
	service.projectName = filepath.Base(projectRoot)
	service.piSessionsDir = t.TempDir()

	planRoot := filepath.Join(
		projectRoot,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
	)
	threadUpdatedAt := time.Now().Add(-2 * time.Minute)
	refs, err := service.buildSessionSidebarRefs([]db.AgentThread{{
		ID:        "thread-1",
		Title:     "Browser thread",
		Cwd:       planRoot,
		UpdatedAt: threadUpdatedAt,
	}})
	if err != nil {
		t.Fatalf("buildSessionSidebarRefs() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("len(refs) before Pi session = %d, want 1", len(refs))
	}

	sessionPath := filepath.Join(service.piSessionsDir, "workspace", "terminal.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","version":3,"id":"pi-session-1","timestamp":"2026-04-21T01:00:00Z","cwd":"`+filepath.ToSlash(
			projectRoot,
		)+`"}`,
		`{"type":"message","id":"user-1","parentId":null,"timestamp":"2026-04-21T01:00:01Z","message":{"role":"user","content":"/q-resume thoughts/creative-mode-agent/plans/2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat/plan.md"}}`,
		`{"type":"custom","id":"custom-1","parentId":"user-1","timestamp":"2026-04-21T01:00:02Z","customType":"plan-classification","data":{"planDir":"thoughts/creative-mode-agent/plans/2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat","source":"prompt-path"}}`,
		`{"type":"session_info","id":"info-1","parentId":"custom-1","timestamp":"2026-04-21T01:00:03Z","name":"[q-resume] terminal handoff"}`,
	)
	now := time.Now()
	if err := os.Chtimes(sessionPath, now, now); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	refs, err = service.buildSessionSidebarRefs([]db.AgentThread{{
		ID:        "thread-1",
		Title:     "Browser thread",
		Cwd:       planRoot,
		UpdatedAt: threadUpdatedAt,
	}})
	if err != nil {
		t.Fatalf("buildSessionSidebarRefs() with Pi session error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("len(refs) = %d, want 2", len(refs))
	}

	groups := service.buildThreadSidebarGroups(refs, "thread-1")
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1", len(groups))
	}
	if groups[0].KindLabel != "Plan" ||
		groups[0].Label != "pkg agents sdk pi temporal datastar chat" ||
		groups[0].Timestamp != "2026-04-19 01-47-47" ||
		groups[0].ThreadCount != 2 ||
		!groups[0].IsActive {
		t.Fatalf("groups[0] = %#v, want active formatted plan group", groups[0])
	}
	if len(groups[0].Threads) != 2 {
		t.Fatalf("len(groups[0].Threads) = %d, want 2", len(groups[0].Threads))
	}
	if groups[0].ThreadCount != len(groups[0].Threads) {
		t.Fatalf(
			"ThreadCount = %d, len Threads = %d",
			groups[0].ThreadCount,
			len(groups[0].Threads),
		)
	}
	hasActiveThread := false
	for _, thread := range groups[0].Threads {
		if thread.ID == "thread-1" && thread.IsActive {
			hasActiveThread = true
		}
	}
	if groups[0].IsActive != hasActiveThread {
		t.Fatalf("group active state did not match active thread")
	}

	var foundPi bool
	var foundBrowser bool
	for _, thread := range groups[0].Threads {
		switch thread.Title {
		case "Browser thread":
			foundBrowser = true
			wantHref := thoughtsDocRedirectURLForRoot(
				service.thoughtsRoot,
				planRoot,
				url.Values{"thread": []string{"thread-1"}},
			)
			if !thread.IsActive || thread.Href != wantHref {
				t.Fatalf("browser thread = %#v, want active thoughts link", thread)
			}
		case "[q-resume] terminal handoff":
			foundPi = true
			if thread.SourceLabel != "Pi" || thread.Href != "" {
				t.Fatalf("pi thread = %#v, want Pi badge and no href", thread)
			}
			if thread.OpenPiSessionAction != piSessionOpenAction() {
				t.Fatalf(
					"pi OpenPiSessionAction = %q, want %q",
					thread.OpenPiSessionAction,
					piSessionOpenAction(),
				)
			}
			if thread.SessionPath != sessionPath {
				t.Fatalf("pi SessionPath = %q, want %q", thread.SessionPath, sessionPath)
			}
			if thread.WorkspaceDir != planRoot {
				t.Fatalf("pi WorkspaceDir = %q, want %q", thread.WorkspaceDir, planRoot)
			}
		}
	}
	if !foundBrowser || !foundPi {
		t.Fatalf(
			"group threads = %#v, want both browser and Pi sessions",
			groups[0].Threads,
		)
	}
}

func TestBuildSessionSidebarRefsFallsBackToPlanDirectoryAncestorForPiSessions(
	t *testing.T,
) {
	service := newTestAgentChatService(t)
	projectRoot := t.TempDir()
	service.projectRoot = projectRoot
	service.projectName = filepath.Base(projectRoot)
	service.piSessionsDir = t.TempDir()

	planRoot := filepath.Join(
		projectRoot,
		"thoughts",
		"creative-mode-agent",
		"plans",
		"2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat",
	)
	sessionPath := filepath.Join(service.piSessionsDir, "workspace", "fallback.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		`{"type":"session","version":3,"id":"pi-session-2","timestamp":"2026-04-21T01:05:00Z","cwd":"`+filepath.ToSlash(
			filepath.Join(planRoot, "research"),
		)+`"}`,
		`{"type":"message","id":"user-1","parentId":null,"timestamp":"2026-04-21T01:05:01Z","message":{"role":"user","content":"/q-question"}}`,
	)

	refs, err := service.buildSessionSidebarRefs(nil)
	if err != nil {
		t.Fatalf("buildSessionSidebarRefs() error = %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("len(refs) = %d, want 1", len(refs))
	}
	if refs[0].Source != SessionThreadSourcePi {
		t.Fatalf("refs[0].Source = %q, want %q", refs[0].Source, SessionThreadSourcePi)
	}
	if refs[0].PlanDir != planRoot {
		t.Fatalf("refs[0].PlanDir = %q, want %q", refs[0].PlanDir, planRoot)
	}

	groups := service.buildThreadSidebarGroups(refs, "")
	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1", len(groups))
	}
	if groups[0].KindLabel != "Plan" ||
		groups[0].Label != "pkg agents sdk pi temporal datastar chat" ||
		groups[0].Timestamp != "2026-04-19 01-47-47" ||
		groups[0].ThreadCount != 1 ||
		groups[0].IsActive {
		t.Fatalf(
			"groups[0] = %#v, want inactive formatted fallback plan group",
			groups[0],
		)
	}
	if len(groups[0].Threads) != 1 || groups[0].Threads[0].CwdLabel != "research" {
		t.Fatalf("groups[0].Threads = %#v, want research cwd label", groups[0].Threads)
	}
	if groups[0].ThreadCount != len(groups[0].Threads) {
		t.Fatalf(
			"ThreadCount = %d, len Threads = %d",
			groups[0].ThreadCount,
			len(groups[0].Threads),
		)
	}
	thread := groups[0].Threads[0]
	if thread.OpenPiSessionAction != piSessionOpenAction() {
		t.Fatalf(
			"OpenPiSessionAction = %q, want %q",
			thread.OpenPiSessionAction,
			piSessionOpenAction(),
		)
	}
	if thread.SessionPath != sessionPath {
		t.Fatalf("SessionPath = %q, want %q", thread.SessionPath, sessionPath)
	}
	if thread.WorkspaceDir != planRoot {
		t.Fatalf("WorkspaceDir = %q, want %q", thread.WorkspaceDir, planRoot)
	}
	hasActiveThread := false
	for _, thread := range groups[0].Threads {
		if thread.ID == "thread-1" && thread.IsActive {
			hasActiveThread = true
		}
	}
	if groups[0].IsActive != hasActiveThread {
		t.Fatalf("group active state did not match active thread")
	}
}

func TestSelectVisibleWorkspaceThread(t *testing.T) {
	t.Parallel()
	workspace := db.Workspace{ID: "workspace-1"}
	threads := []db.AgentThread{
		{ID: "recent", Title: "Recent"},
		{ID: "selected", Title: "Selected"},
	}

	tests := []struct {
		name              string
		activeWorkspaceID string
		selectedThreadID  string
		wantID            string
		wantOK            bool
	}{
		{
			name:              "active selected wins",
			activeWorkspaceID: "workspace-1",
			selectedThreadID:  "selected",
			wantID:            "selected",
			wantOK:            true,
		},
		{
			name:              "active missing selected falls back recent",
			activeWorkspaceID: "workspace-1",
			selectedThreadID:  "missing",
			wantID:            "recent",
			wantOK:            true,
		},
		{
			name:              "inactive uses recent",
			activeWorkspaceID: "other",
			selectedThreadID:  "selected",
			wantID:            "recent",
			wantOK:            true,
		},
		{
			name:              "empty has no visible thread",
			activeWorkspaceID: "workspace-1",
			selectedThreadID:  "selected",
			wantOK:            false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := threads
			if !tt.wantOK {
				input = nil
			}
			got, ok := selectVisibleWorkspaceThread(
				workspace,
				input,
				tt.activeWorkspaceID,
				tt.selectedThreadID,
			)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.ID != tt.wantID {
				t.Fatalf("thread ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}

func TestBuildWorkspaceSidebarStateShowsSingleSelectedThread(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace, recentThread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	selectedThread := mustCreateAgentThread(
		t,
		service,
		"thread-selected",
		"user@example.com",
		workspace.RootDocPath,
		"lineage-selected",
	)
	if err := service.queries.AttachThreadToWorkspace(
		ctx,
		db.AttachThreadToWorkspaceParams{
			ID:          selectedThread.ID,
			WorkspaceID: sql.NullString{String: workspace.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("AttachThreadToWorkspace(selectedThread) error = %v", err)
	}
	if err := service.queries.UpdateWorkspaceSelectedThread(
		ctx,
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspace.ID,
			SelectedThreadID: sql.NullString{String: selectedThread.ID, Valid: true},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceSelectedThread() error = %v", err)
	}
	emptyWorkspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	state := service.BuildWorkspaceSidebarState(
		ctx,
		"user@example.com",
		workspace.ID,
		selectedThread.ID,
	)
	activeGroup := findWorkspaceSidebarGroup(t, state, workspace.ID)
	if activeGroup.ThreadCount != 2 {
		t.Fatalf("active ThreadCount = %d, want 2", activeGroup.ThreadCount)
	}
	if len(activeGroup.Threads) != 1 {
		t.Fatalf("active threads = %#v, want one visible thread", activeGroup.Threads)
	}
	wantThread := ThreadSidebarThread{
		ID:       selectedThread.ID,
		Href:     workspaceThreadHrefForWorkspace(workspace, selectedThread.ID),
		Title:    selectedThread.Title,
		IsActive: true,
	}
	if diff := cmp.Diff(wantThread, activeGroup.Threads[0]); diff != "" {
		t.Fatalf("active thread mismatch (-want +got):\n%s", diff)
	}
	if activeGroup.Threads[0].SourceLabel != "" {
		t.Fatalf("SourceLabel = %q, want empty", activeGroup.Threads[0].SourceLabel)
	}
	if activeGroup.Threads[0].ID == recentThread.ID {
		t.Fatalf(
			"visible thread = recent %q, want selected %q",
			recentThread.ID,
			selectedThread.ID,
		)
	}

	emptyGroup := findWorkspaceSidebarGroup(t, state, emptyWorkspace.ID)
	if emptyGroup.ThreadCount != 0 || len(emptyGroup.Threads) != 0 {
		t.Fatalf("empty group = %#v, want no threads", emptyGroup)
	}
}

func TestWorkspaceSidebarShowsSingleOpenableThreadPerWorkspace(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace, recentThread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	oldThread := mustCreateAgentThread(
		t,
		service,
		"thread-old-sidebar-regression",
		"user@example.com",
		workspace.RootDocPath,
		"lineage-old-sidebar-regression",
	)
	selectedThread := mustCreateAgentThread(
		t,
		service,
		"thread-selected-sidebar-regression",
		"user@example.com",
		workspace.RootDocPath,
		"lineage-selected-sidebar-regression",
	)
	for _, thread := range []db.AgentThread{oldThread, selectedThread} {
		if err := service.queries.AttachThreadToWorkspace(
			ctx,
			db.AttachThreadToWorkspaceParams{
				ID:          thread.ID,
				WorkspaceID: nullString(workspace.ID),
			},
		); err != nil {
			t.Fatalf("AttachThreadToWorkspace(%s) error = %v", thread.ID, err)
		}
	}
	if err := service.queries.UpdateWorkspaceSelectedThread(
		ctx,
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspace.ID,
			SelectedThreadID: nullString(selectedThread.ID),
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceSelectedThread() error = %v", err)
	}

	state := service.BuildWorkspaceSidebarState(
		ctx,
		"user@example.com",
		workspace.ID,
		selectedThread.ID,
	)
	activeGroup := findWorkspaceSidebarGroup(t, state, workspace.ID)
	if activeGroup.ThreadCount != 3 {
		t.Fatalf("ThreadCount = %d, want 3", activeGroup.ThreadCount)
	}
	if len(activeGroup.Threads) != 1 {
		t.Fatalf("visible threads = %#v, want one selected thread", activeGroup.Threads)
	}
	visible := activeGroup.Threads[0]
	if visible.ID != selectedThread.ID {
		t.Fatalf("visible.ID = %q, want selected %q", visible.ID, selectedThread.ID)
	}
	if visible.Href == "" {
		t.Fatalf("visible.Href is empty for selected thread: %#v", visible)
	}
	if visible.ID == recentThread.ID || visible.ID == oldThread.ID {
		t.Fatalf("visible.ID = %q, want selected thread only", visible.ID)
	}
}

func TestWorkspaceSessionHistoryDoesNotDependOnPiPillForClickability(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	workspace := db.Workspace{ID: "workspace-clickability", RootDocPath: "/tmp/project"}
	thread := db.AgentThread{
		ID:        "thread-clickable",
		Title:     "Clickable thread",
		Cwd:       "/tmp/project",
		LineageID: "lineage-clickable",
	}
	sidebarRow := service.workspaceThreadSidebarRow(
		workspace,
		thread,
		workspace.ID,
		thread.ID,
	)
	if sidebarRow.SourceLabel != "" {
		t.Fatalf("sidebar SourceLabel = %q, want empty", sidebarRow.SourceLabel)
	}
	if sidebarRow.Href == "" {
		t.Fatalf("sidebar Href is empty: %#v", sidebarRow)
	}

	threaded := service.workspaceSessionHistoryItem(
		db.AgentSession{
			ID:                  "threaded-session",
			AttachedWorkspaceID: nullString(workspace.ID),
			ProjectedThreadID:   nullString(thread.ID),
			IdentityKind:        "pi",
			ProjectionState:     "hydrated",
		},
		"",
		"",
		WorkspaceSessionSummary{},
	)
	if threaded.SourceLabel != "pi" || threaded.ThreadHref == "" {
		t.Fatalf("threaded history item = %#v, want pi source and thread href", threaded)
	}

	orphan := service.workspaceSessionHistoryItem(
		db.AgentSession{
			ID:                  "orphan-session",
			AttachedWorkspaceID: nullString(workspace.ID),
			ProjectedThreadID:   sql.NullString{},
			IdentityKind:        "pi",
			ProjectionState:     "needs_hydration",
		},
		"",
		"",
		WorkspaceSessionSummary{},
	)
	if orphan.SourceLabel != "pi" || orphan.ThreadHref != "" {
		t.Fatalf("orphan history item = %#v, want pi source without thread href", orphan)
	}
}

func findWorkspaceSidebarGroup(
	t *testing.T,
	state WorkspaceSidebarState,
	workspaceID string,
) ThreadSidebarGroup {
	t.Helper()
	key := "workspace:" + workspaceID
	for _, group := range state.Groups {
		if group.Key == key {
			return group
		}
	}
	t.Fatalf("workspace group %q not found in %#v", key, state.Groups)
	return ThreadSidebarGroup{}
}

func TestFirstCommandFromPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{
			name:   "slash command after prose",
			prompt: "Next: please run /q-plan for the selected outline",
			want:   "/q-plan",
		},
		{
			name:   "backticked slash command",
			prompt: "Finish this with `/q-review` when ready",
			want:   "/q-review",
		},
		{
			name:   "skill token in prose",
			prompt: "Resume the q-implement loop for slice two",
			want:   "q-implement",
		},
		{
			name:   "at command",
			prompt: "Ask @reviewer to check this",
			want:   "@reviewer",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firstCommandFromPrompt(tt.prompt); got != tt.want {
				t.Fatalf("firstCommandFromPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkspaceSessionHistoryItem(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	updatedAt := time.Date(2026, 5, 6, 19, 30, 0, 0, time.UTC)
	session := db.AgentSession{
		ID:                  "session-row-1",
		AttachedWorkspaceID: sql.NullString{String: "workspace-1", Valid: true},
		ProjectedThreadID:   sql.NullString{String: "thread-1", Valid: true},
		IdentityKind:        "pi",
		ArtifactPath:        sql.NullString{String: "/tmp/pi/session.jsonl", Valid: true},
		ExternalSessionID:   sql.NullString{String: "pi-session-1", Valid: true},
		Cwd:                 sql.NullString{String: "/tmp/project", Valid: true},
		ProjectionState:     "hydrated",
		PlanDir:             sql.NullString{String: "thoughts/user/plans/plan", Valid: true},
		LastError:           sql.NullString{String: "warning text", Valid: true},
		UpdatedAt:           updatedAt,
	}
	summary := WorkspaceSessionSummary{
		Header:          PiSessionHeader{ID: "header-session"},
		FirstPromptText: "Please continue with /q-resume from the handoff",
		ImportStats:     PiSessionImportStats{EntriesImported: 3, EntriesSkipped: 1},
		ThreadTitle:     "Thread title",
	}

	got := service.workspaceSessionHistoryItem(session, "thread-1", "", summary)
	want := WorkspaceSessionHistoryItem{
		ID:                 "session-row-1",
		ThreadID:           "thread-1",
		ThreadHref:         threadHref("thread-1"),
		Title:              "header-session",
		Status:             "hydrated",
		SourceLabel:        "pi",
		CwdLabel:           "/tmp/project",
		SessionPathLabel:   "/tmp/pi/session.jsonl",
		InferredPlanLabel:  "thoughts/user/plans/plan",
		FirstPromptExcerpt: "Please continue with /q-resume from the handoff",
		FirstCommandLabel:  "/q-resume",
		ImportStatsLabel:   "3 imported · 1 skipped",
		ErrorLabel:         "warning text",
		UpdatedAtLabel:     "May 6 19:30",
		IsCurrentThread:    true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("workspaceSessionHistoryItem mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildWorkspaceSessionState(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)

	futureMetadata := importMetadataJSONPayload(sessionImportMetadata{
		Inference: WorkspaceInferenceResult{
			Status:  "matched",
			PlanDir: "plans/future",
		},
		Stats:         PiSessionImportStats{EntriesImported: 2},
		Header:        PiSessionHeader{ID: "future-header"},
		FirstUserText: "Start with prose before /q-plan and keep going",
	})
	if _, err := service.queries.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  "session-future",
		ProjectedThreadID:   nullString(thread.ID),
		IndexedByUserEmail:  nullString("user@example.com"),
		IdentityKind:        "global_pi",
		ArtifactPath:        nullString("/tmp/future.jsonl"),
		ExternalSessionID:   nullString("pi-future"),
		ParentSessionID:     sql.NullString{},
		Cwd:                 nullString("/tmp/future"),
		ProjectionState:     "hydrated",
		AttachedWorkspaceID: nullString(workspace.ID),
		PlanDir:             nullString("plans/future"),
		ImportedHeadEntryID: sql.NullString{},
		LastError:           sql.NullString{},
		MetadataJson:        nullString(futureMetadata),
	}); err != nil {
		t.Fatalf("CreateAgentSession(future) error = %v", err)
	}

	oldThread := mustCreateAgentThread(
		t,
		service,
		"thread-old",
		"user@example.com",
		workspace.RootDocPath,
		"lineage-old",
	)
	if err := service.queries.AttachThreadToWorkspace(
		ctx,
		db.AttachThreadToWorkspaceParams{
			ID:          oldThread.ID,
			WorkspaceID: nullString(workspace.ID),
		},
	); err != nil {
		t.Fatalf("AttachThreadToWorkspace(oldThread) error = %v", err)
	}
	mustCreateAgentEntry(
		t,
		service,
		oldThread.LineageID,
		"old-user",
		"",
		"message",
		1,
		`{"type":"message","id":"old-user","message":{"role":"user","content":"Please do q-implement fallback work"}}`,
	)
	if err := service.queries.UpdateAgentThreadHead(ctx, db.UpdateAgentThreadHeadParams{
		ID:          oldThread.ID,
		HeadEntryID: nullString("old-user"),
	}); err != nil {
		t.Fatalf("UpdateAgentThreadHead(oldThread) error = %v", err)
	}
	oldMetadata := importMetadataJSONWithStats(
		WorkspaceInferenceResult{Status: "matched", PlanDir: "plans/old"},
		PiSessionImportStats{EntriesRead: 4},
	)
	if _, err := service.queries.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  "session-old",
		ProjectedThreadID:   nullString(oldThread.ID),
		IndexedByUserEmail:  nullString("user@example.com"),
		IdentityKind:        "global_pi",
		ArtifactPath:        nullString("/tmp/old.jsonl"),
		ExternalSessionID:   nullString("pi-old"),
		ParentSessionID:     sql.NullString{},
		Cwd:                 nullString("/tmp/old"),
		ProjectionState:     "diverged",
		AttachedWorkspaceID: nullString(workspace.ID),
		PlanDir:             nullString("plans/old"),
		ImportedHeadEntryID: nullString("old-user"),
		LastError:           sql.NullString{},
		MetadataJson:        nullString(oldMetadata),
	}); err != nil {
		t.Fatalf("CreateAgentSession(old) error = %v", err)
	}
	if _, err := service.queries.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  "session-malformed",
		ProjectedThreadID:   sql.NullString{},
		IndexedByUserEmail:  nullString("user@example.com"),
		IdentityKind:        "global_pi",
		ArtifactPath:        nullString("/tmp/malformed.jsonl"),
		ExternalSessionID:   nullString("pi-malformed"),
		ParentSessionID:     sql.NullString{},
		Cwd:                 sql.NullString{},
		ProjectionState:     "needs_hydration",
		AttachedWorkspaceID: nullString(workspace.ID),
		PlanDir:             sql.NullString{},
		ImportedHeadEntryID: sql.NullString{},
		LastError:           nullString("bad metadata"),
		MetadataJson:        nullString("{not json"),
	}); err != nil {
		t.Fatalf("CreateAgentSession(malformed) error = %v", err)
	}

	state, err := service.BuildWorkspaceSessionState(ctx, workspace.ID, thread.ID)
	if err != nil {
		t.Fatalf("BuildWorkspaceSessionState() error = %v", err)
	}
	items := map[string]WorkspaceSessionHistoryItem{}
	for _, item := range state.History {
		items[item.ID] = item
	}
	future := items["session-future"]
	if future.Title != "future-header" ||
		future.ThreadHref != workspaceThreadHrefForWorkspace(workspace, thread.ID) ||
		future.FirstCommandLabel != "/q-plan" ||
		future.ImportStatsLabel != "2 imported" ||
		!future.IsCurrentThread {
		t.Fatalf("future item = %#v, want metadata-backed current threaded item", future)
	}
	old := items["session-old"]
	if old.FirstPromptExcerpt != "Please do q-implement fallback work" ||
		old.FirstCommandLabel != "q-implement" || old.ImportStatsLabel != "4 read" ||
		old.ThreadHref != workspaceThreadHrefForWorkspace(workspace, oldThread.ID) {
		t.Fatalf("old item = %#v, want thread-entry prompt fallback", old)
	}
	malformed := items["session-malformed"]
	if malformed.ThreadHref != "" || malformed.Title != "pi-malformed" ||
		malformed.ErrorLabel != "bad metadata" {
		t.Fatalf("malformed item = %#v, want informational fallback row", malformed)
	}
}

func TestBuildPlanSessionStateSelectedPlanOnly(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	selectedPlan := workspace.RootDocPath
	siblingPlan := filepath.Join(filepath.Dir(selectedPlan), "2026-05-01_sibling-plan")
	mustCreateAgentSession(
		t,
		service,
		"session-selected-plan",
		"user@example.com",
		selectedPlan,
		thread.ID,
		workspace.ID,
	)
	mustCreateAgentSession(
		t,
		service,
		"session-sibling-plan",
		"user@example.com",
		siblingPlan,
		"",
		workspace.ID,
	)

	state, err := service.BuildPlanSessionState(
		ctx,
		"user@example.com",
		workspace.ID,
		selectedPlan,
		thread.ID,
		false,
	)
	if err != nil {
		t.Fatalf("BuildPlanSessionState() error = %v", err)
	}
	if state.PlanDir != selectedPlan || state.IncludeDescendants {
		t.Fatalf("state = %#v, want selected plan without descendants", state)
	}
	if len(state.History) != 1 || state.History[0].ID != "session-selected-plan" ||
		!state.History[0].IsCurrentThread {
		t.Fatalf(
			"history = %#v, want selected current-thread session only",
			state.History,
		)
	}
}

func TestBuildPlanSessionStateIncludesDescendants(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	parentPlan := workspace.RootDocPath
	childPlan := filepath.Join(parentPlan, "tickets", "ticket-a")
	siblingPlan := filepath.Join(filepath.Dir(parentPlan), "2026-05-01_sibling-plan")
	mustCreateAgentSession(
		t,
		service,
		"session-parent-plan",
		"user@example.com",
		parentPlan,
		thread.ID,
		workspace.ID,
	)
	mustCreateAgentSession(
		t,
		service,
		"session-child-plan",
		"user@example.com",
		childPlan,
		"",
		workspace.ID,
	)
	mustCreateAgentSession(
		t,
		service,
		"session-sibling-plan",
		"user@example.com",
		siblingPlan,
		"",
		workspace.ID,
	)

	state, err := service.BuildPlanSessionState(
		ctx,
		"user@example.com",
		workspace.ID,
		parentPlan,
		thread.ID,
		true,
	)
	if err != nil {
		t.Fatalf("BuildPlanSessionState() error = %v", err)
	}
	ids := map[string]bool{}
	for _, item := range state.History {
		ids[item.ID] = true
	}
	if !state.IncludeDescendants || !ids["session-parent-plan"] ||
		!ids["session-child-plan"] || ids["session-sibling-plan"] ||
		len(state.History) != 2 {
		t.Fatalf("history ids = %#v, state = %#v; want parent+child only", ids, state)
	}
}

func TestBuildPlanSessionStateFiltersOtherUsersAndWorkspaces(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace, thread := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	otherWorkspace, err := service.CreateWorkspace(ctx, WorkspaceCreateInput{
		UserEmail:   "user@example.com",
		Title:       "Other Workspace",
		RootDocPath: workspace.RootDocPath,
		Cwd:         workspace.RootDocPath,
		Source:      WorkspaceSourceWeb,
	})
	if err != nil {
		t.Fatalf("CreateWorkspace(other) error = %v", err)
	}
	mustCreateAgentSession(
		t,
		service,
		"session-current-workspace",
		"user@example.com",
		workspace.RootDocPath,
		thread.ID,
		workspace.ID,
	)
	mustCreateAgentSession(
		t,
		service,
		"session-other-user",
		"other@example.com",
		workspace.RootDocPath,
		"",
		workspace.ID,
	)
	mustCreateAgentSession(
		t,
		service,
		"session-other-workspace",
		"user@example.com",
		workspace.RootDocPath,
		"",
		otherWorkspace.ID,
	)

	state, err := service.BuildPlanSessionState(
		ctx,
		"user@example.com",
		workspace.ID,
		workspace.RootDocPath,
		thread.ID,
		true,
	)
	if err != nil {
		t.Fatalf("BuildPlanSessionState() error = %v", err)
	}
	if len(state.History) != 1 || state.History[0].ID != "session-current-workspace" {
		t.Fatalf(
			"history = %#v, want current user's active workspace session only",
			state.History,
		)
	}
}

func TestPlanOwnedAgentSessionsSharedAcrossUsers(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace, _ := mustCreateWorkspaceThreadForHandlerTest(
		t,
		service,
		"user@example.com",
	)
	planRel, ok := service.thoughtsRelativePath(workspace.RootDocPath)
	if !ok {
		t.Fatalf("workspace root not under thoughts: %s", workspace.RootDocPath)
	}
	artifactPath := filepath.ToSlash(filepath.Join(planRel, ".sessions", "pi", "session.jsonl"))
	_, err := service.queries.UpsertAgentSessionIndex(ctx, db.UpsertAgentSessionIndexParams{
		ID:                "plan-owned-session",
		IdentityKind:      "plan_owned",
		ArtifactPath:      nullString(artifactPath),
		PlanDir:           nullString(planRel),
		Agent:             "pi",
		FileSize:          1,
		LastIndexedOffset: 1,
		ProjectionState:   "needs_hydration",
	})
	if err != nil {
		t.Fatalf("UpsertAgentSessionIndex: %v", err)
	}

	state, err := service.BuildPlanSessionState(ctx, "other@example.com", "other-workspace", workspace.RootDocPath, "", false)
	if err != nil {
		t.Fatalf("BuildPlanSessionState: %v", err)
	}
	if len(state.History) != 1 || state.History[0].ID != "plan-owned-session" {
		t.Fatalf("history = %#v, want shared plan-owned session", state.History)
	}
}

func TestWorkspaceDocCommentsAreWorkspaceScoped(t *testing.T) {
	service := newTestAgentChatService(t)
	workspaceA := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	workspaceB := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")

	comment, err := service.CreateWorkspaceDocComment(
		context.Background(),
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspaceA.ID,
			DocPath:     "plan.md",
			UserEmail:   "user@example.com",
			CommentText: "Please clarify",
			Anchor:      WorkspaceDocCommentAnchor{SelectedText: "Body"},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}
	wantDocPath := "thoughts/creative-mode-agent/plans/2026-04-30_test-plan/plan.md"
	if !comment.WorkspaceID.Valid || comment.WorkspaceID.String != workspaceA.ID ||
		comment.DocPath != wantDocPath {
		t.Fatalf("comment = %#v, want workspace A %s", comment, wantDocPath)
	}

	commentsA, err := service.ListWorkspaceDocComments(
		context.Background(),
		"user@example.com",
		workspaceA.ID,
		"plan.md",
		false,
	)
	if err != nil {
		t.Fatalf("ListWorkspaceDocComments(A) error = %v", err)
	}
	if len(commentsA) != 1 || commentsA[0].Comment.ID != comment.ID {
		t.Fatalf("commentsA = %#v, want created comment", commentsA)
	}
	commentsB, err := service.ListWorkspaceDocComments(
		context.Background(),
		"user@example.com",
		workspaceB.ID,
		"plan.md",
		false,
	)
	if err != nil {
		t.Fatalf("ListWorkspaceDocComments(B) error = %v", err)
	}
	if len(commentsB) != 1 || commentsB[0].Comment.ID != comment.ID {
		t.Fatalf("commentsB = %#v, want shared canonical document comment", commentsB)
	}
}

func TestWorkspaceDocCommentRepliesResolveReopenAndEvents(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     "plan.md",
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor:      WorkspaceDocCommentAnchor{SelectedText: "Body"},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	reply, err := service.ReplyToWorkspaceDocComment(
		ctx,
		ReplyWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			CommentID:   comment.ID,
			UserEmail:   "agent@example.com",
			ActorType:   "agent",
			ReplyText:   "Handled",
		},
	)
	if err != nil {
		t.Fatalf("ReplyToWorkspaceDocComment() error = %v", err)
	}
	if reply.CommentID != comment.ID || reply.ActorType != "agent" {
		t.Fatalf("reply = %#v, want flat agent reply", reply)
	}
	if err := service.ResolveWorkspaceDocCommentWithRequest(
		ctx,
		"user@example.com",
		workspace.ID,
		comment.ID,
		"user@example.com",
		"user",
		"resolve-1",
	); err != nil {
		t.Fatalf("ResolveWorkspaceDocCommentWithRequest() error = %v", err)
	}
	resolved, err := service.queries.GetWorkspaceDocCommentForWorkspace(
		ctx,
		db.GetWorkspaceDocCommentForWorkspaceParams{
			ID:          comment.ID,
			WorkspaceID: nullString(workspace.ID),
		},
	)
	if err != nil {
		t.Fatalf("GetWorkspaceDocCommentForWorkspace() error = %v", err)
	}
	if !resolved.Resolved {
		t.Fatalf("resolved.Resolved = false, want true")
	}
	if err := service.ReopenWorkspaceDocCommentWithRequest(
		ctx,
		"user@example.com",
		workspace.ID,
		comment.ID,
		"user@example.com",
		"user",
		"reopen-1",
	); err != nil {
		t.Fatalf("ReopenWorkspaceDocCommentWithRequest() error = %v", err)
	}
	reopened, err := service.queries.GetWorkspaceDocCommentForWorkspace(
		ctx,
		db.GetWorkspaceDocCommentForWorkspaceParams{
			ID:          comment.ID,
			WorkspaceID: nullString(workspace.ID),
		},
	)
	if err != nil {
		t.Fatalf("GetWorkspaceDocCommentForWorkspace(reopen) error = %v", err)
	}
	if reopened.Resolved {
		t.Fatalf("reopened.Resolved = true, want false")
	}
	if err := service.ResolveWorkspaceDocCommentWithRequest(
		ctx,
		"user@example.com",
		workspace.ID,
		comment.ID,
		"user@example.com",
		"user",
		"resolve-2",
	); err != nil {
		t.Fatalf("ResolveWorkspaceDocCommentWithRequest(second) error = %v", err)
	}
	events, err := service.queries.ListWorkspaceEvents(
		ctx,
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 20},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	var actionTypes []string
	actionKeys := map[string]bool{}
	for _, event := range events {
		if !event.CommentID.Valid || event.CommentID.String != comment.ID {
			continue
		}
		switch event.EventType {
		case "comment_created", "comment_replied":
			continue
		case "comment_resolved", "comment_reopened":
			actionTypes = append(actionTypes, event.EventType)
			if !event.EventKey.Valid || event.EventKey.String == "" {
				t.Fatalf("action event missing key: %#v", event)
			}
			if actionKeys[event.EventKey.String] {
				t.Fatalf(
					"duplicate action event key %q in events %#v",
					event.EventKey.String,
					events,
				)
			}
			actionKeys[event.EventKey.String] = true
		}
	}
	wantActionTypes := []string{
		"comment_resolved",
		"comment_reopened",
		"comment_resolved",
	}
	if !reflect.DeepEqual(actionTypes, wantActionTypes) {
		t.Fatalf(
			"actionTypes = %#v, want %#v; events = %#v",
			actionTypes,
			wantActionTypes,
			events,
		)
	}
}

func TestWorkspaceDocCommentActionRequestIDIdempotencyAndNoOp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     "plan.md",
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor:      WorkspaceDocCommentAnchor{SelectedText: "Body"},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	for i := range 2 {
		if err := service.ResolveWorkspaceDocCommentWithRequest(
			ctx,
			"user@example.com",
			workspace.ID,
			comment.ID,
			"user@example.com",
			"user",
			"resolve-once",
		); err != nil {
			t.Fatalf("ResolveWorkspaceDocCommentWithRequest(%d) error = %v", i, err)
		}
	}
	if err := service.ResolveWorkspaceDocCommentWithRequest(
		ctx,
		"user@example.com",
		workspace.ID,
		comment.ID,
		"user@example.com",
		"user",
		"resolve-noop",
	); err != nil {
		t.Fatalf("ResolveWorkspaceDocCommentWithRequest(no-op) error = %v", err)
	}

	events, err := service.queries.ListWorkspaceEvents(
		ctx,
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 20},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	resolveEvents := 0
	for _, event := range events {
		if event.EventType == "comment_resolved" && event.CommentID.Valid &&
			event.CommentID.String == comment.ID {
			resolveEvents++
		}
	}
	if resolveEvents != 1 {
		t.Fatalf("resolveEvents = %d, want 1; events = %#v", resolveEvents, events)
	}
}

func TestWorkspaceDocCommentReplyRequestIDIdempotency(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	comment, err := service.CreateWorkspaceDocComment(
		ctx,
		CreateWorkspaceDocCommentInput{
			WorkspaceID: workspace.ID,
			DocPath:     "plan.md",
			UserEmail:   "user@example.com",
			CommentText: "Question",
			Anchor:      WorkspaceDocCommentAnchor{SelectedText: "Body"},
		},
	)
	if err != nil {
		t.Fatalf("CreateWorkspaceDocComment() error = %v", err)
	}

	for i := range 2 {
		_, err := service.ReplyToWorkspaceDocComment(
			ctx,
			ReplyWorkspaceDocCommentInput{
				WorkspaceID: workspace.ID,
				CommentID:   comment.ID,
				UserEmail:   "agent@example.com",
				ActorType:   "agent",
				ReplyText:   "Done",
				RequestID:   "reply-once",
			},
		)
		if err != nil {
			t.Fatalf("ReplyToWorkspaceDocComment(%d) error = %v", i, err)
		}
	}

	replies, err := service.queries.ListWorkspaceDocCommentReplies(
		ctx,
		comment.ID,
	)
	if err != nil {
		t.Fatalf("ListWorkspaceDocCommentReplies() error = %v", err)
	}
	if len(replies) != 1 {
		t.Fatalf("replies = %#v, want one idempotent reply", replies)
	}
}

func TestBuildWorkspaceLogStateFiltersBeforeLimit(t *testing.T) {
	ctx := t.Context()
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	thread := mustCreateAgentThread(
		t,
		service,
		"workspace-log-thread",
		"user@example.com",
		workspace.RootDocPath,
		"workspace-log-lineage",
	)
	if err := service.queries.AttachThreadToWorkspace(
		ctx,
		db.AttachThreadToWorkspaceParams{
			ID:          thread.ID,
			WorkspaceID: nullString(workspace.ID),
		},
	); err != nil {
		t.Fatalf("AttachThreadToWorkspace() error = %v", err)
	}

	if _, err := service.queries.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  "session-1",
		ProjectedThreadID:   nullString(thread.ID),
		IndexedByUserEmail:  nullString("user@example.com"),
		IdentityKind:        "global_pi",
		ArtifactPath:        nullString("/tmp/workspace-log.jsonl"),
		ExternalSessionID:   nullString("pi-session-1"),
		ParentSessionID:     sql.NullString{},
		Cwd:                 nullString(workspace.RootDocPath),
		ProjectionState:     "hydrated",
		AttachedWorkspaceID: nullString(workspace.ID),
		PlanDir:             sql.NullString{},
		ImportedHeadEntryID: sql.NullString{},
		LastError:           sql.NullString{},
		MetadataJson:        sql.NullString{},
	}); err != nil {
		t.Fatalf("CreateAgentSession() error = %v", err)
	}
	mustCreateAgentRun(t, service, thread.ID, "run-1")

	for i := range 5 {
		if _, err := service.AppendWorkspaceEvent(
			ctx,
			service.queries,
			AppendWorkspaceEventInput{
				WorkspaceID: workspace.ID,
				EventType:   "thread_selected",
				ActorType:   "user",
				ThreadID:    thread.ID,
				EventKey:    fmt.Sprintf("excluded-%d", i),
			},
		); err != nil {
			t.Fatalf("AppendWorkspaceEvent(excluded %d) error = %v", i, err)
		}
	}
	included := []AppendWorkspaceEventInput{
		{
			WorkspaceID: workspace.ID,
			EventType:   "artifact_created",
			ActorType:   "system",
			DocPath:     "plan.md",
		},
		{
			WorkspaceID: workspace.ID,
			EventType:   "session_imported",
			ActorType:   "system",
			ThreadID:    thread.ID,
			SessionID:   "session-1",
		},
		{
			WorkspaceID: workspace.ID,
			EventType:   "run_failed",
			ActorType:   "system",
			RunID:       "run-1",
		},
	}
	for _, input := range included {
		if _, err := service.AppendWorkspaceEvent(
			ctx,
			service.queries,
			input,
		); err != nil {
			t.Fatalf("AppendWorkspaceEvent(%s) error = %v", input.EventType, err)
		}
	}

	state, err := service.BuildWorkspaceLogState(ctx, workspace.ID, 2)
	if err != nil {
		t.Fatalf("BuildWorkspaceLogState() error = %v", err)
	}
	if !includeWorkspaceLogEvent("artifact_created") ||
		includeWorkspaceLogEvent("thread_selected") {
		t.Fatalf("includeWorkspaceLogEvent() returned unexpected inclusion values")
	}
	want := []WorkspaceLogItem{
		{
			ID:             state.Events[0].ID,
			Type:           "run_failed",
			Category:       "run",
			Label:          "Run failed",
			Detail:         "run run-1",
			RunID:          "run-1",
			CreatedAtLabel: state.Events[0].CreatedAtLabel,
		},
		{
			ID:             state.Events[1].ID,
			Type:           "session_imported",
			Category:       "session",
			Label:          "Session imported",
			Detail:         "session session-1",
			ThreadID:       thread.ID,
			ThreadHref:     workspaceThreadHrefForWorkspace(workspace, thread.ID),
			SessionID:      "session-1",
			CreatedAtLabel: state.Events[1].CreatedAtLabel,
		},
	}
	if diff := cmp.Diff(want, state.Events); diff != "" {
		t.Fatalf("workspace log mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildWorkspaceMinimapMapsTypedEvents(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	events := []AppendWorkspaceEventInput{
		{
			WorkspaceID: workspace.ID,
			EventType:   "thread_created",
			ActorType:   "user",
		},
		{
			WorkspaceID: workspace.ID,
			EventType:   "artifact_updated",
			ActorType:   "system",
			DocPath:     "design.md",
		},
		{
			WorkspaceID: workspace.ID,
			EventType:   "workflow_stage_changed",
			ActorType:   "system",
			PayloadJSON: `{"current_step":"design","status":"review"}`,
		},
	}
	for _, input := range events {
		if _, err := service.AppendWorkspaceEvent(
			context.Background(),
			service.queries,
			input,
		); err != nil {
			t.Fatalf("AppendWorkspaceEvent(%s) error = %v", input.EventType, err)
		}
	}

	state, err := service.BuildWorkspaceMinimap(context.Background(), workspace.ID, 10)
	if err != nil {
		t.Fatalf("BuildWorkspaceMinimap() error = %v", err)
	}
	if len(state.Events) != 3 {
		t.Fatalf("events = %#v, want three", state.Events)
	}
	checks := []struct {
		index    int
		category string
		label    string
	}{
		{0, "thread", "Thread created"},
		{1, "artifact", "Artifact updated: design.md"},
		{2, "workflow", "Workflow: design (review)"},
	}
	for _, check := range checks {
		got := state.Events[check.index]
		if got.Category != check.category || got.Label != check.label {
			t.Fatalf(
				"event %d = (%q, %q), want (%q, %q)",
				check.index,
				got.Category,
				got.Label,
				check.category,
				check.label,
			)
		}
	}
}

func TestBuildWorkspaceWorkflowStateDefaultsQRSPIToDefinitionGraph(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	if err := service.queries.UpdateWorkspaceWorkflowState(
		context.Background(),
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      string(WorkspaceWorkflowQRSPI),
			WorkflowStateJson: sql.NullString{},
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}
	updated, err := service.queries.GetWorkspace(context.Background(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}

	projected, err := service.BuildWorkspaceWorkflowState(context.Background(), updated)
	if err != nil {
		t.Fatalf("BuildWorkspaceWorkflowState() error = %v", err)
	}
	if strings.Contains(projected.Mermaid, "review-design") ||
		!strings.Contains(projected.Mermaid, "human-review-implementation") {
		t.Fatalf(
			"Mermaid = %q, want definition-derived QRSPI review nodes",
			projected.Mermaid,
		)
	}
}

func TestBuildWorkspaceWorkflowStateProjectsRuntimeState(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	runtimeState := wruntime.State{
		Type:          string(WorkspaceWorkflowQRSPI),
		Version:       "v1",
		CurrentNodeID: wruntime.NodeID("human-review-outline"),
		Status:        wruntime.WorkspaceStatusWaitingHuman,
		Policy: json.RawMessage(
			`{"autoMode":false,"enablePlanReviews":false,"invalidResultRetryLimit":1}`,
		),
		Attempts: map[wruntime.NodeID]int{
			wruntime.NodeID("outline"):        1,
			wruntime.NodeID("review-outline"): 0,
		},
		Nodes: map[wruntime.NodeID]wruntime.NodeState{
			wruntime.NodeID("review-plan"): {
				Status:       wruntime.NodeStatusBypassed,
				BypassReason: "plan reviews disabled",
			},
			wruntime.NodeID("outline"): {
				Status:       wruntime.NodeStatusComplete,
				LastArtifact: "outline.md",
			},
		},
		LastResult: &wruntime.WorkflowResultSnapshot{
			SourceNodeID:    wruntime.NodeID("outline"),
			Status:          "complete",
			Summary:         "Outline reviewed and ready for human approval.",
			PrimaryArtifact: "thoughts/example/outline.md",
			DisplayNext:     "/q-review thoughts/example/outline.md",
		},
		HumanGate: &wruntime.HumanGateState{
			From:                wruntime.NodeID("review-outline"),
			To:                  wruntime.NodeID("human-review-outline"),
			Reason:              "outline approved by human",
			ReviewContextNodeID: wruntime.NodeID("review-outline"),
			ReviewArtifact:      "thoughts/example/reviews/review.md",
		},
	}
	raw, err := json.Marshal(runtimeState)
	if err != nil {
		t.Fatalf("Marshal(runtimeState): %v", err)
	}
	if err := service.queries.UpdateWorkspaceWorkflowState(
		context.Background(),
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      string(WorkspaceWorkflowQRSPI),
			WorkflowStateJson: nullString(string(raw)),
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}
	updated, err := service.queries.GetWorkspace(context.Background(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}

	projected, err := service.BuildWorkspaceWorkflowState(context.Background(), updated)
	if err != nil {
		t.Fatalf("BuildWorkspaceWorkflowState() error = %v", err)
	}
	if projected.Type != WorkspaceWorkflowQRSPI ||
		projected.CurrentStep != "human-review-outline" ||
		projected.Status != string(wruntime.WorkspaceStatusWaitingHuman) ||
		!projected.WaitingHuman ||
		projected.ReviewGate != "human-review-outline" ||
		projected.HumanGateReason != "outline approved by human" ||
		projected.LastResultSummary != "Outline reviewed and ready for human approval." ||
		projected.PrimaryArtifact != "thoughts/example/outline.md" ||
		projected.NextDisplay != "/q-review thoughts/example/outline.md" {
		t.Fatalf("projected = %#v, want runtime workflow projection", projected)
	}
	if diff := cmp.Diff(
		[]string{"review-plan"},
		projected.BypassedNodes,
	); diff != "" {
		t.Fatalf("BypassedNodes mismatch (-want +got):\n%s", diff)
	}
	if !strings.Contains(projected.Mermaid, "review-outline") ||
		!strings.Contains(projected.Mermaid, "human-review-implementation") {
		t.Fatalf("Mermaid = %q, want definition-derived QRSPI graph", projected.Mermaid)
	}
}

func TestWorkspaceCwdProjectionBlocksImplementationWithoutExecutionCwd(t *testing.T) {
	t.Parallel()

	got := ProjectWorkspaceCwd(wruntime.State{
		Type:          string(WorkspaceWorkflowQRSPI),
		CurrentNodeID: qrspi.NodeImplement,
	}, db.Workspace{Cwd: nullString("/tmp/planning")})
	if !got.Blocked || got.Scope != "implementation_workspace" ||
		!strings.Contains(got.BlockReason, "/q-workspace") {
		t.Fatalf("ProjectWorkspaceCwd() = %#v, want blocked implementation cwd", got)
	}
}

func TestWorkspaceCwdProjectionUsesExecutionCwdForImplementation(t *testing.T) {
	t.Parallel()

	executionCwd := "/tmp/implementation-copy"
	got := ProjectWorkspaceCwd(wruntime.State{
		Type:          string(WorkspaceWorkflowQRSPI),
		CurrentNodeID: qrspi.NodeReviewImplementation,
		ExecutionCwd:  executionCwd,
	}, db.Workspace{Cwd: nullString("/tmp/planning")})
	if got.Blocked || got.Path != executionCwd || got.Label != "implementation-copy" ||
		got.Scope != "implementation_workspace" {
		t.Fatalf("ProjectWorkspaceCwd() = %#v, want implementation cwd", got)
	}
}

func TestProjectWorkspaceWorkflowPolicyLabelsAndTiming(t *testing.T) {
	raw := mustMarshalJSON(t, qrspi.Policy{
		AutoMode:                true,
		EnablePlanReviews:       false,
		InvalidResultRetryLimit: 2,
	})
	got, err := ProjectWorkspaceWorkflowPolicy(wruntime.State{
		Type:   string(WorkspaceWorkflowQRSPI),
		Status: wruntime.WorkspaceStatusWaitingHuman,
		Policy: raw,
	})
	if err != nil {
		t.Fatalf("ProjectWorkspaceWorkflowPolicy() error = %v", err)
	}
	if !got.AutoMode || got.EnablePlanReviews || got.InvalidResultRetryLimit != 2 {
		t.Fatalf("policy = %#v", got)
	}
	if got.Preset != "" {
		t.Fatalf("Preset = %q, want custom policy preset", got.Preset)
	}
	if !strings.Contains(got.TimingCopy, "Current gate target") {
		t.Fatalf("TimingCopy = %q", got.TimingCopy)
	}
	if !got.Editable {
		t.Fatalf("Editable = false, want waiting-human policy editable")
	}
	if !strings.Contains(got.AdvancedJSON, "invalidResultRetryLimit") {
		t.Fatalf("AdvancedJSON = %q, want policy JSON", got.AdvancedJSON)
	}
}

func TestUpdateWorkspaceWorkflowPolicyPersistsValidatedRuntimeStateAndEvent(
	t *testing.T,
) {
	service := newTestAgentChatService(t)
	workspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.DefaultPolicy(),
		wruntime.WorkspaceStatusWaitingHuman,
	)

	got, err := service.UpdateWorkspaceWorkflowPolicy(
		context.Background(),
		UpdateWorkspaceWorkflowPolicyInput{
			WorkspaceID:             workspace.ID,
			UserEmail:               "user@example.com",
			AutoMode:                true,
			EnablePlanReviews:       false,
			InvalidResultRetryLimit: 0,
		},
	)
	if err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowPolicy() error = %v", err)
	}
	if !got.Policy.AutoMode || got.Policy.EnablePlanReviews ||
		got.Policy.InvalidResultRetryLimit != 0 {
		t.Fatalf("projection = %#v", got.Policy)
	}

	updated, err := service.queries.GetWorkspace(context.Background(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	var runtimeState wruntime.State
	if err := json.Unmarshal(
		[]byte(updated.WorkflowStateJson.String),
		&runtimeState,
	); err != nil {
		t.Fatalf("Unmarshal(runtimeState): %v", err)
	}
	policy := qrspi.ParsePolicy(runtimeState.Policy)
	if !policy.AutoMode || policy.EnablePlanReviews ||
		policy.InvalidResultRetryLimit != 0 {
		t.Fatalf("policy = %#v", policy)
	}
	events, err := service.queries.ListWorkspaceEvents(
		context.Background(),
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	if !hasWorkspaceEventType(events, "workflow_policy_updated") {
		t.Fatalf("missing workflow_policy_updated in %#v", events)
	}
	if category, label := workspaceEventCategoryAndLabel(
		events[len(events)-1],
	); category != "workflow" ||
		label != "Workflow policy updated" {
		t.Fatalf("workspaceEventCategoryAndLabel() = %q, %q", category, label)
	}
}

func TestUpdateWorkspaceWorkflowPolicyAppendsEventForEveryUpdate(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.DefaultPolicy(),
		wruntime.WorkspaceStatusIdle,
	)

	for i := 0; i < 2; i++ {
		_, err := service.UpdateWorkspaceWorkflowPolicy(
			context.Background(),
			UpdateWorkspaceWorkflowPolicyInput{
				WorkspaceID:             workspace.ID,
				UserEmail:               "user@example.com",
				AutoMode:                i%2 == 0,
				EnablePlanReviews:       true,
				InvalidResultRetryLimit: 1,
			},
		)
		if err != nil {
			t.Fatalf("UpdateWorkspaceWorkflowPolicy(%d) error = %v", i, err)
		}
	}
	events, err := service.queries.ListWorkspaceEvents(
		context.Background(),
		db.ListWorkspaceEventsParams{WorkspaceID: workspace.ID, Limit: 10},
	)
	if err != nil {
		t.Fatalf("ListWorkspaceEvents() error = %v", err)
	}
	count := 0
	keys := map[string]bool{}
	for _, event := range events {
		if event.EventType != "workflow_policy_updated" {
			continue
		}
		count++
		if !event.EventKey.Valid || event.EventKey.String == "" {
			t.Fatalf("event key missing for %#v", event)
		}
		if keys[event.EventKey.String] {
			t.Fatalf("duplicate event key %q", event.EventKey.String)
		}
		keys[event.EventKey.String] = true
	}
	if count != 2 {
		t.Fatalf("workflow_policy_updated count = %d, want 2", count)
	}
}

func TestUpdateWorkspaceWorkflowPolicyRejectsDoneAndInvalidRetry(t *testing.T) {
	service := newTestAgentChatService(t)
	doneWorkspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.DefaultPolicy(),
		wruntime.WorkspaceStatusDone,
	)
	if _, err := service.UpdateWorkspaceWorkflowPolicy(
		context.Background(),
		UpdateWorkspaceWorkflowPolicyInput{
			WorkspaceID:             doneWorkspace.ID,
			UserEmail:               "user@example.com",
			AutoMode:                true,
			EnablePlanReviews:       true,
			InvalidResultRetryLimit: 1,
		},
	); err == nil ||
		!strings.Contains(err.Error(), "read-only") {
		t.Fatalf("UpdateWorkspaceWorkflowPolicy(done) error = %v, want read-only", err)
	}

	activeWorkspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.DefaultPolicy(),
		wruntime.WorkspaceStatusIdle,
	)
	if _, err := service.UpdateWorkspaceWorkflowPolicy(
		context.Background(),
		UpdateWorkspaceWorkflowPolicyInput{
			WorkspaceID:             activeWorkspace.ID,
			UserEmail:               "user@example.com",
			AutoMode:                true,
			EnablePlanReviews:       true,
			InvalidResultRetryLimit: -1,
		},
	); err == nil ||
		!strings.Contains(err.Error(), "non-negative") {
		t.Fatalf(
			"UpdateWorkspaceWorkflowPolicy(invalid retry) error = %v, want non-negative",
			err,
		)
	}
}

func TestBuildWorkspaceWorkflowStateProjectsPolicy(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateQRSPIWorkspaceWithRuntimeState(
		t,
		service,
		"user@example.com",
		qrspi.Policy{
			AutoMode:                true,
			EnablePlanReviews:       false,
			InvalidResultRetryLimit: 1,
		},
		wruntime.WorkspaceStatusRunning,
	)

	updated, err := service.queries.GetWorkspace(context.Background(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	projected, err := service.BuildWorkspaceWorkflowState(context.Background(), updated)
	if err != nil {
		t.Fatalf("BuildWorkspaceWorkflowState() error = %v", err)
	}
	if projected.Policy.Preset != WorkflowPolicyPresetFastDraft ||
		projected.Policy.ModeLabel == "" ||
		projected.Policy.ReviewLabel == "" {
		t.Fatalf("Policy projection = %#v, want fast draft labels", projected.Policy)
	}
	if !strings.Contains(projected.Policy.TimingCopy, "current agent run") {
		t.Fatalf("TimingCopy = %q, want running copy", projected.Policy.TimingCopy)
	}
}

func TestUpdateWorkspaceWorkflowStateProjectsQRSPIState(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	state := WorkspaceWorkflowState{
		Type:        WorkspaceWorkflowQRSPI,
		CurrentStep: "design",
		Status:      "review",
		ReviewGate:  "waiting",
		Mermaid:     "flowchart LR\n  design --> review",
		Metadata:    json.RawMessage(`{"workflow_run_id":"workflow-1"}`),
	}
	if err := service.UpdateWorkspaceWorkflowState(
		context.Background(),
		workspace.ID,
		state,
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}

	updated, err := service.queries.GetWorkspace(context.Background(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	projected, err := service.BuildWorkspaceWorkflowState(context.Background(), updated)
	if err != nil {
		t.Fatalf("BuildWorkspaceWorkflowState() error = %v", err)
	}
	if projected.Type != WorkspaceWorkflowQRSPI ||
		projected.CurrentStep != "design" ||
		projected.Status != "review" ||
		projected.ReviewGate != "waiting" ||
		projected.Mermaid == "" {
		t.Fatalf("projected = %#v, want persisted QRSPI state", projected)
	}
	minimap, err := service.BuildWorkspaceMinimap(context.Background(), workspace.ID, 10)
	if err != nil {
		t.Fatalf("BuildWorkspaceMinimap() error = %v", err)
	}
	if len(minimap.Events) != 1 || minimap.Events[0].Category != "workflow" {
		t.Fatalf("minimap = %#v, want workflow event", minimap.Events)
	}
	if service.CurrentCursor(workspace.ID) == 0 {
		t.Fatalf("CurrentCursor() = 0, want workflow update notification")
	}
}

func TestReconcileUnattachedAgentChatThreadsAttachesThreads(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "thoughts")
	planRoot := filepath.Join(
		thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_workspace",
	)
	if err := os.MkdirAll(planRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(planRoot): %v", err)
	}
	service.thoughtsRoot = thoughtsRoot
	thread := mustCreateAgentThread(
		t,
		service,
		"historical-thread",
		"user@example.com",
		planRoot,
		"historical-lineage",
	)

	if err := service.ReconcileUnattachedAgentChatThreads(
		context.Background(),
		"user@example.com",
	); err != nil {
		t.Fatalf("ReconcileUnattachedAgentChatThreads() error = %v", err)
	}
	primary, err := service.queries.GetPrimaryWorkspaceForThread(context.Background(), db.GetPrimaryWorkspaceForThreadParams{ThreadID: thread.ID, UserEmail: "user@example.com"})
	if err != nil {
		t.Fatalf("GetPrimaryWorkspaceForThread() error = %v", err)
	}
	if primary.ID == "" {
		t.Fatalf("primary.ID empty, want attached workspace")
	}
}

func TestWorkspacePatchScopeForEventRefreshesResource(t *testing.T) {
	t.Parallel()

	for _, eventType := range []string{
		"thread_created",
		"thread_selected",
		"thread_forked",
		"thread_attached",
		"run_started",
		"run_checkpointed",
		"run_completed",
		"run_failed",
		"artifact_selected",
		"artifact_created",
		"artifact_updated",
		"artifact_deleted",
		"comment_created",
		"comment_replied",
		"comment_resolved",
		"comment_reopened",
		"session_imported",
		"session_import_diverged",
		"session_sync_failed",
		"workflow_stage_changed",
		"unknown_future_event",
	} {
		t.Run(eventType, func(t *testing.T) {
			got := WorkspacePatchScopeForEvent(db.WorkspaceEvent{EventType: eventType})
			if got != PatchWorkspaceResource {
				t.Fatalf(
					"WorkspacePatchScopeForEvent(%q) = %q, want %q",
					eventType,
					got,
					PatchWorkspaceResource,
				)
			}
		})
	}
}

func TestNotifyWorkspaceForEventEmitsOneResourceCursor(t *testing.T) {
	service := newTestAgentChatService(t)
	workspace := mustCreateWorkspaceForHandlerTest(t, service, "user@example.com")
	sub := service.notifier.Subscribe(workspace.ID)
	defer service.notifier.Unsubscribe(workspace.ID, sub)

	service.NotifyWorkspaceForEvent(db.WorkspaceEvent{
		WorkspaceID: workspace.ID,
		EventType:   "comment_resolved",
	})

	select {
	case signal := <-sub:
		if signal.Scope != PatchWorkspaceResource {
			t.Fatalf("signal.Scope = %q, want %q", signal.Scope, PatchWorkspaceResource)
		}
		if signal.Cursor != 1 {
			t.Fatalf("signal.Cursor = %d, want 1", signal.Cursor)
		}
	default:
		t.Fatal("subscriber did not receive resource signal")
	}
	select {
	case extra := <-sub:
		t.Fatalf("unexpected extra signal: %#v", extra)
	default:
	}
}

func TestPlanDirectoryRootFindsNearestPlanAncestor(t *testing.T) {
	path := "/tmp/project/thoughts/creative-mode-agent/plans/2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat/research/notes.md"
	want := "/tmp/project/thoughts/creative-mode-agent/plans/2026-04-19_01-47-47_pkg-agents-sdk-pi-temporal-datastar-chat"
	if got := planDirectoryRoot(path); got != want {
		t.Fatalf("planDirectoryRoot(%q) = %q, want %q", path, got, want)
	}
	if got := planDirectoryRoot("/tmp/project/server/services"); got != "" {
		t.Fatalf("planDirectoryRoot(non-plan path) = %q, want empty", got)
	}
}

func TestServiceCallbackURLUsesConfiguredBase(t *testing.T) {
	svc := &Service{callbackBaseURL: "http://127.0.0.1:4301/"}
	got := svc.callbackURL("/internal/agent-chat/events")
	want := "http://127.0.0.1:4301/internal/agent-chat/events"
	if got != want {
		t.Fatalf("callbackURL() = %q, want %q", got, want)
	}

	got = (&Service{}).callbackURL("internal/agent-chat/events")
	want = "http://localhost:4200/internal/agent-chat/events"
	if got != want {
		t.Fatalf("fallback callbackURL() = %q, want %q", got, want)
	}
}

func mustMarshalJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%T): %v", value, err)
	}
	return json.RawMessage(encoded)
}

func mustCreateQRSPIWorkspaceWithRuntimeState(
	t *testing.T,
	service *Service,
	userEmail string,
	policy qrspi.Policy,
	status wruntime.WorkspaceStatus,
) db.Workspace {
	t.Helper()
	workspace := mustCreateWorkspaceForHandlerTest(t, service, userEmail)
	state := wruntime.State{
		Type:          string(WorkspaceWorkflowQRSPI),
		Version:       "v1",
		CurrentNodeID: wruntime.NodeID("question"),
		Status:        status,
		Policy:        mustMarshalJSON(t, policy),
		Attempts:      map[wruntime.NodeID]int{},
		Nodes: map[wruntime.NodeID]wruntime.NodeState{
			wruntime.NodeID("question"): {Status: wruntime.NodeStatusPending},
		},
	}
	raw := mustMarshalJSON(t, state)
	if err := service.queries.UpdateWorkspaceWorkflowState(
		context.Background(),
		db.UpdateWorkspaceWorkflowStateParams{
			ID:                workspace.ID,
			WorkflowType:      string(WorkspaceWorkflowQRSPI),
			WorkflowStateJson: nullString(string(raw)),
		},
	); err != nil {
		t.Fatalf("UpdateWorkspaceWorkflowState() error = %v", err)
	}
	updated, err := service.queries.GetWorkspace(context.Background(), workspace.ID)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	return updated
}

func hasWorkspaceEventType(events []db.WorkspaceEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}

func newAgentChatTestDB(t *testing.T) (*sql.DB, *db.Queries) {
	t.Helper()

	dbConn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "agentchat.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })

	schemaPath := filepath.Join("..", "..", "..", "pkg", "db", "migrations", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("ReadFile(schema): %v", err)
	}
	if _, err := dbConn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("Exec(foreign_keys): %v", err)
	}
	if _, err := dbConn.Exec(string(schema)); err != nil {
		t.Fatalf("Exec(schema): %v", err)
	}
	return dbConn, db.New(dbConn)
}

func TestWorkspaceThreadCwdPrefersConfiguredDefaultCwd(t *testing.T) {
	service := &Service{
		projectRoot: "/host/thoughts-repo",
		defaultCwd:  "/workspace/checkout",
	}

	got := service.workspaceThreadCwd(db.Workspace{
		RootDocPath: "/host/thoughts-repo/thoughts",
	})
	if got != "/workspace/checkout" {
		t.Fatalf("workspaceThreadCwd() = %q, want default cwd", got)
	}
}

func TestNewServiceWithOptionsUsesConfiguredThoughtsRoot(t *testing.T) {
	dbConn, queries := newAgentChatTestDB(t)
	hostRepo := filepath.Join(t.TempDir(), "host-repo")
	codeCheckout := filepath.Join(t.TempDir(), "vamos-main")
	thoughtsRoot := filepath.Join(hostRepo, "thoughts")

	service, err := NewServiceWithOptions(
		dbConn,
		queries,
		NewNotifier(),
		nil,
		nil,
		ServiceOptions{
			ProjectRoot:  hostRepo,
			ProjectName:  "vamos",
			DefaultCwd:   codeCheckout,
			ThoughtsRoot: thoughtsRoot,
			ImplWorkspaceDiscovery: workspaces.ImplWorkspaceDiscoveryConfig{
				MainCheckoutPath: codeCheckout,
			},
		},
	)
	if err != nil {
		t.Fatalf("NewServiceWithOptions() error = %v", err)
	}

	input := service.PlanWorkspaceDiscoveryInput()
	if input.ThoughtsRoot != thoughtsRoot {
		t.Fatalf("ThoughtsRoot = %q, want %q", input.ThoughtsRoot, thoughtsRoot)
	}
	if input.ProjectRoot != hostRepo {
		t.Fatalf("ProjectRoot = %q, want %q", input.ProjectRoot, hostRepo)
	}
	if input.ImplWorkspaces.MainCheckoutPath != codeCheckout {
		t.Fatalf(
			"MainCheckoutPath = %q, want %q",
			input.ImplWorkspaces.MainCheckoutPath,
			codeCheckout,
		)
	}
}

func newTestAgentChatService(t *testing.T) *Service {
	t.Helper()

	dbConn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "agentchat.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })

	schemaPath := filepath.Join("..", "..", "..", "pkg", "db", "migrations", "schema.sql")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("ReadFile(schema): %v", err)
	}
	if _, err := dbConn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("Exec(foreign_keys): %v", err)
	}
	if _, err := dbConn.Exec(string(schema)); err != nil {
		t.Fatalf("Exec(schema): %v", err)
	}

	service, err := NewService(
		dbConn,
		db.New(dbConn),
		NewNotifier(),
		nil,
		nil,
		"/tmp/project",
		"/tmp/project",
		10,
		"",
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	service.piSessionsDir = t.TempDir()
	return service
}

func mustUpsertDiscoveredPlanWorkspace(
	t *testing.T,
	service *Service,
	planDir string,
	label string,
	artifactUpdatedAt time.Time,
) db.PlanWorkspace {
	t.Helper()
	return mustUpsertDiscoveredPlanWorkspaceWithProject(t, service, planDir, label, "", artifactUpdatedAt)
}

func mustUpsertDiscoveredPlanWorkspaceWithProject(
	t *testing.T,
	service *Service,
	planDir string,
	label string,
	projectID string,
	artifactUpdatedAt time.Time,
) db.PlanWorkspace {
	t.Helper()
	rel, err := planWorkspaceRel(service.thoughtsRoot, planDir)
	if err != nil {
		t.Fatalf("planWorkspaceRel() error = %v", err)
	}
	row, err := service.queries.UpsertDiscoveredPlanWorkspace(
		t.Context(),
		db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        rel,
			ProjectID:         projectID,
			PlanDir:           planDir,
			Label:             label,
			ArtifactUpdatedAt: artifactUpdatedAt,
		},
	)
	if err != nil {
		t.Fatalf("UpsertDiscoveredPlanWorkspace() error = %v", err)
	}
	return row
}

func mustCreateAgentSession(
	t *testing.T,
	service *Service,
	id string,
	userEmail string,
	planDir string,
	threadID string,
	workspaceID string,
) db.AgentSession {
	t.Helper()
	storedPlanDir := planDir
	if rel, ok := service.thoughtsRelativePath(planDir); ok {
		storedPlanDir = rel
	}
	session, err := service.queries.CreateAgentSession(
		t.Context(),
		db.CreateAgentSessionParams{
			ID:                  id,
			AttachedWorkspaceID: sql.NullString{String: workspaceID, Valid: workspaceID != ""},
			ProjectedThreadID:   sql.NullString{String: threadID, Valid: threadID != ""},
			IndexedByUserEmail: sql.NullString{
				String: userEmail,
				Valid:  userEmail != "",
			},
			IdentityKind: "global_pi",
			ArtifactPath: sql.NullString{
				String: filepath.Join(t.TempDir(), id+".jsonl"),
				Valid:  true,
			},
			ExternalSessionID:   sql.NullString{String: id, Valid: true},
			ParentSessionID:     sql.NullString{},
			Cwd:                 sql.NullString{String: planDir, Valid: planDir != ""},
			ProjectionState:     "unassigned",
			PlanDir:             sql.NullString{String: storedPlanDir, Valid: storedPlanDir != ""},
			ImportedHeadEntryID: sql.NullString{},
			LastError:           sql.NullString{},
			MetadataJson:        sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentSession: %v", err)
	}
	return session
}

func writePiSessionFile(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(session dir): %v", err)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(session): %v", err)
	}
}

func flattenPlanSidebarNodeLabels(nodes []PlanSidebarNode) []string {
	labels := make([]string, 0, len(nodes))
	for _, node := range nodes {
		labels = append(labels, node.Label)
		labels = append(labels, flattenPlanSidebarNodeLabels(node.Children)...)
	}
	return labels
}

func mustCreateAgentThread(
	t *testing.T,
	service *Service,
	id, userEmail, cwd, lineageID string,
) db.AgentThread {
	t.Helper()
	thread, err := service.queries.CreateAgentThread(
		context.Background(),
		db.CreateAgentThreadParams{
			ID:                id,
			UserEmail:         userEmail,
			Title:             "Test Thread",
			Cwd:               cwd,
			LineageID:         lineageID,
			HeadEntryID:       sql.NullString{},
			ParentThreadID:    sql.NullString{},
			ForkedFromEntryID: sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentThread: %v", err)
	}
	return thread
}

func mustCreateAgentRun(
	t *testing.T,
	service *Service,
	threadID, runID string,
) db.AgentRun {
	t.Helper()
	return mustCreateAgentRunWithRoot(t, service, runID, threadID, "/tmp/project")
}

func mustCreateAgentRunWithRoot(
	t *testing.T,
	service *Service,
	runID, threadID, root string,
) db.AgentRun {
	t.Helper()
	run, err := service.queries.CreateAgentRun(
		context.Background(),
		db.CreateAgentRunParams{
			ID:                 runID,
			WorkspaceID:        sql.NullString{},
			ThreadID:           threadID,
			SessionID:          sql.NullString{},
			Trigger:            "send",
			Status:             "running",
			PromptText:         "hello",
			RestoreHeadEntryID: sql.NullString{},
			ResultHeadEntryID:  sql.NullString{},
			WorkflowID:         "workflow-" + runID,
			TemporalRunID:      sql.NullString{},
			RootDocPath:        root,
			ErrorMessage:       sql.NullString{},
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentRun: %v", err)
	}
	return run
}

func mustCreateAgentEntry(
	t *testing.T,
	service *Service,
	lineageID, entryID, parentEntryID, entryType string,
	originOrder int64,
	payload string,
) {
	t.Helper()
	err := service.queries.CreateAgentEntry(
		context.Background(),
		db.CreateAgentEntryParams{
			LineageID: lineageID,
			EntryID:   entryID,
			ParentEntryID: sql.NullString{
				String: parentEntryID,
				Valid:  parentEntryID != "",
			},
			EntryType:        entryType,
			OriginOrder:      originOrder,
			PayloadJson:      payload,
			OriginThreadID:   "thread-1",
			OriginRunID:      sql.NullString{},
			OriginSessionID:  sql.NullString{},
			SessionTimestamp: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("CreateAgentEntry: %v", err)
	}
}

func readNotifierScopes(
	t *testing.T,
	ch chan ThreadStreamSignal,
	count int,
) []StreamPatchScope {
	t.Helper()

	scopes := make([]StreamPatchScope, 0, count)
	timeout := time.After(time.Second)
	for len(scopes) < count {
		select {
		case signal := <-ch:
			scopes = append(scopes, signal.Scope)
		case <-timeout:
			t.Fatalf("timed out waiting for %d notifier scopes, got %v", count, scopes)
		}
	}
	return scopes
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
