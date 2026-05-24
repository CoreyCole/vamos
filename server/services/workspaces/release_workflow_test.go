package workspaces

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/release"
)

func TestTemporalReleaseStarterStartsAndSignalsQueue(t *testing.T) {
	client := &fakeReleaseTemporalClient{}
	starter := NewTemporalReleaseStarter(client)
	if err := starter.EnqueueRelease(context.Background(), "item-1"); err != nil {
		t.Fatalf("enqueue release: %v", err)
	}
	if client.startedID != "release-queue/default" {
		t.Fatalf("started workflow id=%q", client.startedID)
	}
	if client.signalName != ReleaseQueueSignalName || client.signaledID != "release-queue/default" {
		t.Fatalf("signal id/name=%q/%q", client.signaledID, client.signalName)
	}
	sig, ok := client.signalArg.(ReleaseQueueSignal)
	if !ok || sig.ItemID != "item-1" {
		t.Fatalf("signal arg=%#v", client.signalArg)
	}
}

func TestReleaseActivitiesProcessItemsInDBClaimOrderAfterFailure(t *testing.T) {
	store := newFakeReleaseStore([]ReleaseQueueItem{
		newReleaseQueueTestItem("item-1"),
		newReleaseQueueTestItem("item-2"),
	})
	exec := &fakeReleaseExecutor{failNode: "merge"}
	activities := newTestReleaseActivities(store, exec)

	processed, err := activities.ProcessNextReleaseQueueItem(context.Background())
	if err != nil || !processed {
		t.Fatalf("first process processed=%t err=%v", processed, err)
	}
	if store.items["item-1"].Status != ReleaseQueueStatusFailed {
		t.Fatalf("item-1 status=%s", store.items["item-1"].Status)
	}
	exec.failNode = ""
	processed, err = activities.ProcessNextReleaseQueueItem(context.Background())
	if err != nil || !processed {
		t.Fatalf("second process processed=%t err=%v", processed, err)
	}
	if store.items["item-2"].Status != ReleaseQueueStatusSucceeded {
		t.Fatalf("item-2 status=%s", store.items["item-2"].Status)
	}
	if !reflect.DeepEqual(store.claimed, []string{"item-1", "item-2"}) {
		t.Fatalf("claimed=%v", store.claimed)
	}
}

func TestReleaseActivitiesExecuteServiceNodesInGraphOrder(t *testing.T) {
	store := newFakeReleaseStore([]ReleaseQueueItem{newReleaseQueueTestItem("item-1")})
	exec := &fakeReleaseExecutor{}
	activities := newTestReleaseActivities(store, exec)

	processed, err := activities.ProcessNextReleaseQueueItem(context.Background())
	if err != nil || !processed {
		t.Fatalf("process processed=%t err=%v", processed, err)
	}
	want := []runtime.NodeID{"preflight", "merge"}
	if !reflect.DeepEqual(exec.nodes, want) {
		t.Fatalf("executed nodes=%v want %v", exec.nodes, want)
	}
	if !reflect.DeepEqual(store.runningNodes, want) {
		t.Fatalf("running nodes=%v want %v", store.runningNodes, want)
	}
}

func newTestReleaseActivities(store *fakeReleaseStore, exec *fakeReleaseExecutor) *ReleaseActivities {
	workflowRegistry := runtime.NewRegistry()
	wf := runtime.New[struct{}]("release.promote").
		Start("preflight").
		Service("preflight", runtime.ServiceSpec{Type: "release.preflight"}).
		Service("merge", runtime.ServiceSpec{Type: "release.merge"}).
		Done("done").
		Edge("preflight", "merge").
		Edge("merge", "done").
		MustBuild()
	if err := workflowRegistry.Register(wf); err != nil {
		panic(err)
	}
	releaseRegistry := release.NewRegistry(workflowRegistry)
	def := release.NewDefinition("default").
		Lane("stage", release.CheckoutSlug("stage"), release.Label("Stage")).
		Flow("promote", "release.promote", release.FromFeature(), release.ToLane("stage"), release.NoPush()).
		MustBuild(workflowRegistry)
	if err := releaseRegistry.Register(def); err != nil {
		panic(err)
	}
	return &ReleaseActivities{Store: store, ReleaseRegistry: releaseRegistry, WorkflowRegistry: workflowRegistry, Executor: exec}
}

func newReleaseQueueTestItem(id string) ReleaseQueueItem {
	return ReleaseQueueItem{
		ID:                id,
		DefinitionID:      "default",
		DefinitionVersion: "v1",
		WorkflowID:        "release.promote",
		WorkflowVersion:   "v1",
		FlowID:            "promote",
		Status:            ReleaseQueueStatusPending,
	}
}

type fakeReleaseTemporalClient struct {
	startedID  string
	signaledID string
	signalName string
	signalArg  any
}

func (f *fakeReleaseTemporalClient) StartWorkflowIdempotent(ctx context.Context, workflowID string, workflowFunc, input any) (string, error) {
	f.startedID = workflowID
	return "run-1", nil
}

func (f *fakeReleaseTemporalClient) SignalWorkflow(ctx context.Context, workflowID, runID, signalName string, arg any) error {
	f.signaledID = workflowID
	f.signalName = signalName
	f.signalArg = arg
	return nil
}

type fakeReleaseExecutor struct {
	failNode runtime.NodeID
	nodes    []runtime.NodeID
}

func (f *fakeReleaseExecutor) ExecuteReleaseNode(ctx context.Context, item ReleaseQueueItem, def release.Definition, flow release.FlowDefinition, node runtime.Node, onLine func(string)) error {
	f.nodes = append(f.nodes, node.ID)
	onLine("log " + string(node.ID))
	if node.ID == f.failNode {
		return errors.New("node failed")
	}
	return nil
}

type fakeReleaseStore struct {
	queue        []string
	items        map[string]ReleaseQueueItem
	claimed      []string
	runningNodes []runtime.NodeID
	events       []AppendReleaseQueueEventParams
}

func newFakeReleaseStore(items []ReleaseQueueItem) *fakeReleaseStore {
	store := &fakeReleaseStore{items: map[string]ReleaseQueueItem{}}
	for _, item := range items {
		store.queue = append(store.queue, item.ID)
		store.items[item.ID] = item
	}
	return store
}

func (s *fakeReleaseStore) CreateReleaseQueueItem(ctx context.Context, arg CreateReleaseQueueItemParams) (ReleaseQueueItem, error) {
	item := ReleaseQueueItem{ID: arg.ID, DefinitionID: arg.DefinitionID, DefinitionVersion: arg.DefinitionVersion, WorkflowID: arg.WorkflowID, WorkflowVersion: arg.WorkflowVersion, FlowID: arg.FlowID, Status: ReleaseQueueStatusPending}
	s.items[item.ID] = item
	s.queue = append(s.queue, item.ID)
	return item, nil
}
func (s *fakeReleaseStore) GetReleaseQueueItem(ctx context.Context, id string) (ReleaseQueueItem, error) {
	return s.items[id], nil
}
func (s *fakeReleaseStore) ListActiveReleaseQueueItems(ctx context.Context) ([]ReleaseQueueItem, error) {
	return nil, nil
}
func (s *fakeReleaseStore) ListRecentReleaseQueueItems(ctx context.Context, limit int) ([]ReleaseQueueItem, error) {
	return nil, nil
}
func (s *fakeReleaseStore) ClaimNextPendingReleaseQueueItem(ctx context.Context) (ReleaseQueueItem, bool, error) {
	for len(s.queue) > 0 {
		id := s.queue[0]
		s.queue = s.queue[1:]
		item := s.items[id]
		if item.Status == ReleaseQueueStatusPending {
			item.Status = ReleaseQueueStatusRunning
			s.items[id] = item
			s.claimed = append(s.claimed, id)
			return item, true, nil
		}
	}
	return ReleaseQueueItem{}, false, nil
}
func (s *fakeReleaseStore) MarkReleaseQueueItemRunning(ctx context.Context, id string, node runtime.NodeID) error {
	item := s.items[id]
	item.Status = ReleaseQueueStatusRunning
	item.CurrentNodeID = node
	s.items[id] = item
	s.runningNodes = append(s.runningNodes, node)
	return nil
}
func (s *fakeReleaseStore) MarkReleaseQueueItemTerminal(ctx context.Context, id string, status ReleaseQueueStatus, errMsg string) error {
	item := s.items[id]
	item.Status = status
	item.ErrorMessage = errMsg
	s.items[id] = item
	return nil
}
func (s *fakeReleaseStore) AppendReleaseQueueEvent(ctx context.Context, arg AppendReleaseQueueEventParams) error {
	s.events = append(s.events, arg)
	return nil
}
func (s *fakeReleaseStore) ListReleaseQueueEvents(ctx context.Context, itemID string, limit int) ([]ReleaseQueueEvent, error) {
	return nil, nil
}
