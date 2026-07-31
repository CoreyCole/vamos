package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/server/services/agentchat"
)

type registrationRecorder struct {
	workflows  []string
	activities []string
}

func (r *registrationRecorder) RegisterWorkflow(value any) {
	r.workflows = append(r.workflows, registeredValueName(value))
}

func (r *registrationRecorder) RegisterActivity(value any) {
	r.activities = append(r.activities, registeredValueName(value))
}

func registeredValueName(value any) string {
	reflected := reflect.ValueOf(value)
	if reflected.Kind() == reflect.Func {
		return runtime.FuncForPC(reflected.Pointer()).Name()
	}
	return fmt.Sprintf("%T", value)
}

func TestAgentChatBootstrapDoesNotRegisterOpaqueSettlementDiscovery(t *testing.T) {
	t.Parallel()

	recorder := &registrationRecorder{}
	registerAgentChatTemporalWorker(
		recorder,
		&agentchat.Service{},
		&agentchat.SyncCoordinator{},
		&agentchat.WorkspaceSyncer{},
		&agentchat.WorkspaceSyncGuard{},
	)

	registered := strings.Join(
		append(append([]string{}, recorder.workflows...), recorder.activities...),
		"\n",
	)
	for _, retired := range []string{
		"OpaqueSettlementDiscoveryWorkflow",
		"DeliverOpaqueSettlements",
		"opaque-settlement-discovery:",
	} {
		if strings.Contains(registered, retired) {
			t.Fatalf("retired registration %q found in:\n%s", retired, registered)
		}
	}
	if !strings.Contains(registered, "SyncCoordinatorWorkflow") ||
		!strings.Contains(registered, "SyncWorkspacesWorkflow") {
		t.Fatalf("workspace sync registration missing from:\n%s", registered)
	}
}
