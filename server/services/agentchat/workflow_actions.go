package agentchat

import "strings"

func workflowPolicyAction(state WorkspaceWorkflowState) string {
	if threadID := strings.TrimSpace(state.ThreadID); threadID != "" {
		return "@post('/agent-chat/thread/" + threadID + "/workflow/policy', {contentType: 'form'})"
	}
	return "@post('/agent-chat/" + state.WorkspaceID + "/workflow/policy', {contentType: 'form'})"
}

func workflowAdvanceAction(state WorkspaceWorkflowState) string {
	if threadID := strings.TrimSpace(state.ThreadID); threadID != "" {
		return "@post('/agent-chat/thread/" + threadID + "/workflow/advance', {contentType: 'form'})"
	}
	return "@post('/agent-chat/" + state.WorkspaceID + "/workflow/advance', {contentType: 'form'})"
}
