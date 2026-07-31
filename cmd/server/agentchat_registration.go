package main

import "github.com/CoreyCole/vamos/server/services/agentchat"

type temporalRegistrar interface {
	RegisterWorkflow(any)
	RegisterActivity(any)
}

func registerAgentChatTemporalWorker(
	worker temporalRegistrar,
	service *agentchat.Service,
	coordinator *agentchat.SyncCoordinator,
	syncer *agentchat.WorkspaceSyncer,
	guard *agentchat.WorkspaceSyncGuard,
) {
	worker.RegisterWorkflow(agentchat.SyncCoordinatorWorkflow)
	worker.RegisterWorkflow(agentchat.SyncWorkspacesWorkflow)
	worker.RegisterActivity(service.FailConversationRunAfterActivityError)
	worker.RegisterActivity(&agentchat.SyncCoordinatorActivities{
		Coordinator: coordinator,
	})
	worker.RegisterActivity(&agentchat.WorkspaceSyncActivities{
		Syncer: syncer,
	})
	worker.RegisterActivity(&agentchat.PlanWorkspaceDiscoveryActivities{
		Syncer: service.PlanWorkspaceDiscoverySyncer(),
		Guard:  guard,
	})
}
