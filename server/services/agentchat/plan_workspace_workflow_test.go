package agentchat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"

	temporalmgr "github.com/CoreyCole/vamos/pkg/agents/temporal"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

func TestSyncWorkspacesWorkflowRunsSyncActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	calls := 0
	wantInput := SyncWorkspacesInput{
		ProjectName:        "cn-agents",
		ProjectInstanceKey: "cn-agents-abc123",
		ProjectRoot:        "/tmp/cn-agents",
		ThoughtsRoot:       "/tmp/cn-agents/thoughts",
		ImplWorkspaces: workspaces.ImplWorkspaceDiscoveryConfig{
			MainCheckoutPath: "/tmp/cn-agents",
			ParentDir:        "/tmp",
			Domain:           "workspaces.test",
		},
		ManagerURL:   "https://main.workspaces.test",
		RestartToken: "token",
		TrunkBranch:  "main",
	}
	wantResult := SyncWorkspacesResult{
		Plan: PlanWorkspaceDiscoveryResult{
			Scanned:    3,
			Discovered: 2,
			Upserted:   2,
			Changed:    true,
		},
		Impl: workspaces.ImplWorkspaceSyncResult{
			Scanned:     2,
			Discovered:  2,
			Upserted:    2,
			RepairedEnv: 1,
			Changed:     true,
		},
		Changed: true,
	}
	env.RegisterActivityWithOptions(
		func(input SyncWorkspacesInput) (SyncWorkspacesResult, error) {
			calls++
			if !reflect.DeepEqual(input, wantInput) {
				t.Fatalf("input=%#v want %#v", input, wantInput)
			}
			return wantResult, nil
		},
		activity.RegisterOptions{Name: "SyncWorkspaces"},
	)

	env.ExecuteWorkflow(SyncWorkspacesWorkflow, wantInput)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error=%v", err)
	}
	var got SyncWorkspacesResult
	if err := env.GetWorkflowResult(&got); err != nil {
		t.Fatalf("GetWorkflowResult() error = %v", err)
	}
	if got != wantResult {
		t.Fatalf("workflow result=%#v want %#v", got, wantResult)
	}
	if calls != 1 {
		t.Fatalf("activity calls=%d want 1", calls)
	}
}

func TestSyncWorkspacesActivityRequiresSyncer(t *testing.T) {
	_, err := (&WorkspaceSyncActivities{}).SyncWorkspaces(
		context.Background(),
		SyncWorkspacesInput{},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "workspace sync activity requires syncer") {
		t.Fatalf("SyncWorkspaces() error = %v, want workspace syncer", err)
	}
}

type fakeWorkspaceSyncRunner struct {
	result SyncWorkspacesResult
	err    error
}

func (f fakeWorkspaceSyncRunner) Sync(context.Context, SyncWorkspacesInput) (SyncWorkspacesResult, error) {
	return f.result, f.err
}

func TestWorkspaceSyncActivityCallsCompletionHookWhenEnabled(t *testing.T) {
	wantResult := SyncWorkspacesResult{Changed: true}
	var called bool
	var gotResult SyncWorkspacesResult
	var gotErr error
	activity := &WorkspaceSyncActivities{
		Syncer: fakeWorkspaceSyncRunner{result: wantResult},
		OnComplete: func(_ context.Context, result SyncWorkspacesResult, err error) {
			called = true
			gotResult = result
			gotErr = err
		},
	}

	result, err := activity.SyncWorkspaces(
		context.Background(),
		SyncWorkspacesInput{RunCompletionHook: true},
	)
	if err != nil {
		t.Fatalf("SyncWorkspaces() error = %v", err)
	}
	if result != wantResult || gotResult != wantResult || gotErr != nil || !called {
		t.Fatalf(
			"completion called=%t result=%#v got=%#v err=%v",
			called,
			result,
			gotResult,
			gotErr,
		)
	}
}

func TestWorkspaceSyncActivitySkipsCompletionHookWhenDisabled(t *testing.T) {
	activity := &WorkspaceSyncActivities{
		Syncer: fakeWorkspaceSyncRunner{result: SyncWorkspacesResult{Changed: true}},
		OnComplete: func(context.Context, SyncWorkspacesResult, error) {
			t.Fatal("completion hook should not run when disabled")
		},
	}

	if _, err := activity.SyncWorkspaces(context.Background(), SyncWorkspacesInput{}); err != nil {
		t.Fatalf("SyncWorkspaces() error = %v", err)
	}
}

func TestWorkspaceSyncerRunsPlanAndImplSync(t *testing.T) {
	service := newTestAgentChatService(t)
	root := t.TempDir()
	thoughtsRoot := filepath.Join(root, "cn-agents", "thoughts")
	parent := filepath.Join(root, "checkouts")
	mainCheckout := filepath.Join(parent, "cn-agents")
	featureCheckout := filepath.Join(parent, "cn-agents-feature")
	for _, checkout := range []string{mainCheckout, featureCheckout} {
		if err := os.MkdirAll(
			filepath.Join(checkout, "pkg", "agents"),
			0o755,
		); err != nil {
			t.Fatalf("MkdirAll(%s): %v", checkout, err)
		}
		if err := os.WriteFile(
			filepath.Join(checkout, "pkg", "agents", "go.mod"),
			[]byte("module test\n"),
			0o644,
		); err != nil {
			t.Fatalf("WriteFile(go.mod): %v", err)
		}
	}
	writePlanWorkspaceFile(
		t,
		filepath.Join(thoughtsRoot, "agent", "plans", "sync-test"),
		"plan.md",
		time.Now(),
	)

	input := SyncWorkspacesInput{
		ProjectName:        "cn-agents",
		ProjectInstanceKey: "cn-agents-test",
		ProjectRoot:        filepath.Join(root, "cn-agents"),
		ThoughtsRoot:       thoughtsRoot,
		ImplWorkspaces: workspaces.ImplWorkspaceDiscoveryConfig{
			MainCheckoutPath: mainCheckout,
			ParentDir:        parent,
			Domain:           "workspaces.test",
			CheckoutPrefixes: []string{"cn-agents"},
			MainCheckoutName: "cn-agents",
			PackageSubdir:    "pkg/agents",
		},
		ManagerURL:   "https://main.workspaces.test",
		RestartToken: "secret",
		TrunkBranch:  "main",
	}
	result, err := (&WorkspaceSyncer{
		PlanSyncer: &PlanWorkspaceSyncer{
			Queries: service.queries,
			Scanner: PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot},
		},
		ImplSyncer: &workspaces.ImplWorkspaceSyncer{Queries: service.queries},
	}).Sync(context.Background(), input)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Plan.Upserted != 1 || result.Impl.Upserted != 2 || !result.Changed {
		t.Fatalf("Sync() result = %#v", result)
	}
	if _, err := service.queries.GetPlanWorkspace(
		context.Background(),
		"agent/plans/sync-test",
	); err != nil {
		t.Fatalf("GetPlanWorkspace: %v", err)
	}
	if _, err := service.queries.GetImplWorkspace(
		context.Background(),
		db.GetImplWorkspaceParams{WorkspaceSlug: "feature"},
	); err != nil {
		t.Fatalf("GetImplWorkspace(feature): %v", err)
	}
}

func TestProjectInstanceKeyAndSyncWorkspacesScheduleIDsAreDeterministic(t *testing.T) {
	keyA1 := projectInstanceKey("CN Agents", "/tmp/a/cn-agents")
	keyA2 := projectInstanceKey("CN Agents", "/tmp/a/cn-agents")
	keyB := projectInstanceKey("CN Agents", "/tmp/b/cn-agents")
	if keyA1 != keyA2 {
		t.Fatalf("projectInstanceKey() not deterministic: %q != %q", keyA1, keyA2)
	}
	if keyA1 == keyB {
		t.Fatalf("projectInstanceKey() should differ for distinct roots: %q", keyA1)
	}
	if !strings.HasPrefix(
		SyncWorkspacesScheduleID(keyA1),
		"agent-chat-sync-workspaces:",
	) {
		t.Fatalf("schedule ID = %q", SyncWorkspacesScheduleID(keyA1))
	}
	if got := SyncWorkspacesWorkflowID(
		keyA1,
	); got != SyncWorkspacesScheduleID(
		keyA1,
	)+":sync" {
		t.Fatalf("workflow ID = %q", got)
	}
}

func TestEnsureSyncWorkspacesScheduleShape(t *testing.T) {
	input := SyncWorkspacesInput{
		ProjectName:        "CN Agents",
		ProjectInstanceKey: "CN Agents / Workspace",
		ProjectRoot:        "/tmp/cn-agents",
		ThoughtsRoot:       "/tmp/cn-agents/thoughts",
	}
	if defaultWorkspaceSyncInterval != time.Minute {
		t.Fatalf(
			"defaultWorkspaceSyncInterval = %v, want %v",
			defaultWorkspaceSyncInterval,
			time.Minute,
		)
	}

	schedule := syncWorkspacesSchedule(input)
	if schedule.Spec == nil || len(schedule.Spec.Intervals) != 1 ||
		schedule.Spec.Intervals[0].Every != time.Minute {
		t.Fatalf("schedule spec = %#v", schedule.Spec)
	}
	if schedule.Policy == nil ||
		schedule.Policy.Overlap != enumspb.SCHEDULE_OVERLAP_POLICY_SKIP {
		t.Fatalf("schedule policy = %#v", schedule.Policy)
	}
	scheduleAction, ok := schedule.Action.(*client.ScheduleWorkflowAction)
	if !ok || scheduleAction == nil {
		t.Fatalf("schedule action = %#v", schedule.Action)
	}
	if scheduleAction.ID != SyncWorkspacesWorkflowID(input.ProjectInstanceKey) {
		t.Fatalf("action ID = %q", scheduleAction.ID)
	}
	if scheduleAction.TaskQueue != temporalmgr.GoTaskQueue {
		t.Fatalf(
			"task queue = %q, want %q",
			scheduleAction.TaskQueue,
			temporalmgr.GoTaskQueue,
		)
	}
	if len(scheduleAction.Args) != 1 {
		t.Fatalf("action args = %#v", scheduleAction.Args)
	}
	if got, ok := scheduleAction.Args[0].(SyncWorkspacesInput); !ok ||
		!reflect.DeepEqual(got, input) {
		t.Fatalf("action args[0] = %#v", scheduleAction.Args[0])
	}
}

func TestWorkspaceSyncInputIncludesRuntimeConfig(t *testing.T) {
	t.Parallel()
	service := &Service{
		projectName:  "project",
		projectRoot:  "/tmp/cn-agents",
		thoughtsRoot: "/tmp/cn-agents/thoughts",
		implWorkspaceDiscovery: workspaces.ImplWorkspaceDiscoveryConfig{
			ParentDir: "/tmp",
			Domain:    "workspaces.test",
		},
	}
	service.SetWorkspaceRuntimeConfig(
		" https://main.workspaces.test/ ",
		" restart-token ",
	)
	input := service.WorkspaceSyncInput()
	if input.ManagerURL != "https://main.workspaces.test/" ||
		input.RestartToken != "restart-token" {
		t.Fatalf(
			"runtime config = manager %q token %q",
			input.ManagerURL,
			input.RestartToken,
		)
	}
	if input.ImplWorkspaces.MainCheckoutPath != "/tmp/cn-agents" ||
		input.TrunkBranch != "main" {
		t.Fatalf("workspace sync input = %#v", input)
	}
}

func TestPlanWorkspaceDiscoveryWorkflowRunsSyncActivity(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	calls := 0
	wantInput := PlanWorkspaceDiscoveryInput{
		ProjectName:        "cn-agents",
		ProjectInstanceKey: "cn-agents-abc123",
		ProjectRoot:        "/tmp/cn-agents",
		ThoughtsRoot:       "/tmp/cn-agents/thoughts",
	}
	wantResult := PlanWorkspaceDiscoveryResult{
		Scanned:              3,
		Discovered:           2,
		Upserted:             2,
		Archived:             1,
		Restored:             1,
		Changed:              true,
		MaxArtifactUpdatedAt: time.Date(2026, 5, 16, 20, 0, 0, 0, time.UTC),
	}
	env.RegisterActivityWithOptions(
		func(input PlanWorkspaceDiscoveryInput) (PlanWorkspaceDiscoveryResult, error) {
			calls++
			if !reflect.DeepEqual(input, wantInput) {
				t.Fatalf("input=%#v want %#v", input, wantInput)
			}
			return wantResult, nil
		},
		activity.RegisterOptions{Name: "SyncPlanWorkspaces"},
	)

	env.ExecuteWorkflow(PlanWorkspaceDiscoveryWorkflow, wantInput)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error=%v", err)
	}
	var got PlanWorkspaceDiscoveryResult
	if err := env.GetWorkflowResult(&got); err != nil {
		t.Fatalf("GetWorkflowResult() error = %v", err)
	}
	if got != wantResult {
		t.Fatalf("workflow result=%#v want %#v", got, wantResult)
	}
	if calls != 1 {
		t.Fatalf("activity calls=%d want 1", calls)
	}
}

func TestPlanWorkspaceDiscoveryActivityRequiresSyncer(t *testing.T) {
	_, err := (&PlanWorkspaceDiscoveryActivities{}).SyncPlanWorkspaces(
		context.Background(),
		PlanWorkspaceDiscoveryInput{},
	)
	if err == nil || !strings.Contains(err.Error(), "requires syncer") {
		t.Fatalf("SyncPlanWorkspaces() error = %v, want requires syncer", err)
	}
}

func TestPlanWorkspaceDiscoveryActivityHonorsCanceledContext(t *testing.T) {
	service := newTestAgentChatService(t)
	thoughtsRoot := t.TempDir()
	planDir := filepath.Join(thoughtsRoot, "agent", "plans", "cancel-test")
	writePlanWorkspaceFile(t, planDir, "plan.md", time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&PlanWorkspaceDiscoveryActivities{
		Syncer: &PlanWorkspaceSyncer{
			Queries:  service.queries,
			Scanner:  PlanWorkspaceScanner{ThoughtsRoot: thoughtsRoot},
			Notifier: service,
		},
	}).SyncPlanWorkspaces(ctx, PlanWorkspaceDiscoveryInput{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SyncPlanWorkspaces() error = %v, want context.Canceled", err)
	}
}

func TestProjectInstanceKeyAndScheduleIDsAreDeterministic(t *testing.T) {
	keyA1 := projectInstanceKey("CN Agents", "/tmp/a/cn-agents")
	keyA2 := projectInstanceKey("CN Agents", "/tmp/a/cn-agents")
	keyB := projectInstanceKey("CN Agents", "/tmp/b/cn-agents")
	if keyA1 != keyA2 {
		t.Fatalf("projectInstanceKey() not deterministic: %q != %q", keyA1, keyA2)
	}
	if keyA1 == keyB {
		t.Fatalf("projectInstanceKey() should differ for distinct roots: %q", keyA1)
	}
	if !strings.HasPrefix(
		PlanWorkspaceDiscoveryScheduleID(keyA1),
		"agent-chat-plan-workspace-discovery:",
	) {
		t.Fatalf("schedule ID = %q", PlanWorkspaceDiscoveryScheduleID(keyA1))
	}
	if got := PlanWorkspaceDiscoveryWorkflowID(
		keyA1,
	); got != PlanWorkspaceDiscoveryScheduleID(
		keyA1,
	)+":scan" {
		t.Fatalf("workflow ID = %q", got)
	}
}

func TestPlanWorkspaceDiscoveryInputUsesDefaultCwdForInstanceKey(t *testing.T) {
	t.Parallel()
	serviceA := &Service{
		projectName:  "project",
		defaultCwd:   "/tmp/a/cn-agents/pkg/agents",
		thoughtsRoot: "/tmp/a/cn-agents/thoughts",
	}
	serviceB := &Service{
		projectName:  "project",
		defaultCwd:   "/tmp/b/cn-agents/pkg/agents",
		thoughtsRoot: "/tmp/b/cn-agents/thoughts",
	}
	inputA := serviceA.PlanWorkspaceDiscoveryInput()
	inputB := serviceB.PlanWorkspaceDiscoveryInput()
	if inputA.ProjectInstanceKey == inputB.ProjectInstanceKey {
		t.Fatalf(
			"ProjectInstanceKey should differ for distinct default cwd values: %q",
			inputA.ProjectInstanceKey,
		)
	}
}

func TestEnsurePlanWorkspaceDiscoveryScheduleShape(t *testing.T) {
	input := PlanWorkspaceDiscoveryInput{
		ProjectName:        "CN Agents",
		ProjectInstanceKey: "CN Agents / Workspace",
		ProjectRoot:        "/tmp/cn-agents",
		ThoughtsRoot:       "/tmp/cn-agents/thoughts",
	}
	if defaultPlanWorkspaceDiscoveryInterval != time.Minute {
		t.Fatalf(
			"defaultPlanWorkspaceDiscoveryInterval = %v, want %v",
			defaultPlanWorkspaceDiscoveryInterval,
			time.Minute,
		)
	}

	schedule := planWorkspaceDiscoverySchedule(input)
	if schedule.Spec == nil || len(schedule.Spec.Intervals) != 1 ||
		schedule.Spec.Intervals[0].Every != time.Minute {
		t.Fatalf("schedule spec = %#v", schedule.Spec)
	}
	if schedule.Policy == nil ||
		schedule.Policy.Overlap != enumspb.SCHEDULE_OVERLAP_POLICY_SKIP {
		t.Fatalf("schedule policy = %#v", schedule.Policy)
	}
	scheduleAction, ok := schedule.Action.(*client.ScheduleWorkflowAction)
	if !ok || scheduleAction == nil {
		t.Fatalf("schedule action = %#v", schedule.Action)
	}
	if scheduleAction.ID != PlanWorkspaceDiscoveryWorkflowID(input.ProjectInstanceKey) {
		t.Fatalf("action ID = %q", scheduleAction.ID)
	}
	if scheduleAction.TaskQueue != temporalmgr.GoTaskQueue {
		t.Fatalf(
			"task queue = %q, want %q",
			scheduleAction.TaskQueue,
			temporalmgr.GoTaskQueue,
		)
	}
	if len(scheduleAction.Args) != 1 {
		t.Fatalf("action args = %#v", scheduleAction.Args)
	}
	if got, ok := scheduleAction.Args[0].(PlanWorkspaceDiscoveryInput); !ok ||
		!reflect.DeepEqual(got, input) {
		t.Fatalf("action args[0] = %#v", scheduleAction.Args[0])
	}
}
