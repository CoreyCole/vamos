package agentchat

import (
	"strings"

	workspace "github.com/CoreyCole/vamos/pkg/agents/workspace"
	"github.com/CoreyCole/vamos/pkg/db"
)

func WorkspacePatchScopeForEvent(event db.WorkspaceEvent) WorkspacePatchScope {
	return WorkspacePatchScope(workspace.ScopeForPersistedEvent(event.EventType))
}

func (s *Service) NotifyWorkspaceForEvent(event db.WorkspaceEvent) {
	if s.notifier == nil || strings.TrimSpace(event.WorkspaceID) == "" {
		return
	}
	s.notifier.NotifyWorkspaceResource(event.WorkspaceID)
}
