package agentchat

import (
	"fmt"
	"hash/fnv"
	"net/url"
	"strconv"
	"strings"

	agentworkspace "github.com/CoreyCole/vamos/pkg/agents/workspace"
	"github.com/CoreyCole/vamos/pkg/components/sheet"
	"github.com/CoreyCole/vamos/pkg/components/utils"
)

const (
	piSessionOpenEndpoint = "/agent-chat/pi-sessions/open"

	agentChatMobilePaneChat = "chat"
	agentChatMobilePaneRail = "rail"
	agentChatThreadSheetID  = "agent_chat_thread_sheet"
)

type agentChatShellSignalOptions struct {
	IncludeRightRailTab bool
	SidebarOpenGroup    string
	SidebarOpenGroups   string
}

func freeformForkAction(threadID string) string {
	if strings.TrimSpace(threadID) == "" {
		return ""
	}
	return fmt.Sprintf(
		"@post('%s', {contentType: 'form'})",
		threadChatAction(threadID, "fork"),
	)
}

func threadChatAction(threadID, action string) string {
	threadID = strings.TrimSpace(threadID)
	action = strings.Trim(strings.TrimSpace(action), "/")
	if threadID == "" || action == "" {
		return ""
	}
	return "/agent-chat/thread/" + url.PathEscape(threadID) + "/" + action
}

func piSessionOpenAction() string {
	return "@post('" + piSessionOpenEndpoint + "', {contentType: 'form'})"
}

func piSessionOpenIndicator(thread ThreadSidebarThread) string {
	key := strings.TrimSpace(thread.SessionPath)
	if key == "" {
		key = strings.TrimSpace(thread.Title)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("_openingPiSession%x", h.Sum32())
}

func piSessionOpenIndicatorSignal(thread ThreadSidebarThread) string {
	return "$" + piSessionOpenIndicator(thread)
}

func workspaceForkAction(workspaceID, threadID string) string {
	if threadID != "" {
		return fmt.Sprintf(
			"@post('%s', {contentType: 'form'})",
			threadChatAction(threadID, "fork"),
		)
	}
	return freeformForkAction(threadID)
}

func workspaceSendAction(workspaceID, threadID string, hasThread bool) string {
	if hasThread {
		return fmt.Sprintf(
			"@post('%s', {contentType: 'form'})",
			threadChatAction(threadID, "resume"),
		)
	}
	return fmt.Sprintf(
		"@post('/agent-chat/%s/send', {contentType: 'form'})",
		url.PathEscape(workspaceID),
	)
}

func thoughtsThreadChatAction(threadID, action string) string {
	threadID = strings.TrimSpace(threadID)
	action = strings.Trim(strings.TrimSpace(action), "/")
	if threadID == "" || action == "" {
		return ""
	}
	return "/thoughts/chat/thread/" + url.PathEscape(threadID) + "/" + action
}

func embeddedWorkspaceSendAction(workspaceID, threadID string, hasThread bool) string {
	if hasThread {
		return fmt.Sprintf(
			"@post('%s', {contentType: 'form'})",
			thoughtsThreadChatAction(threadID, "resume"),
		)
	}
	return fmt.Sprintf(
		"@post('/thoughts/chat/%s/send', {contentType: 'form'})",
		url.PathEscape(workspaceID),
	)
}

func embeddedAttachDocAction(workspaceID, threadID string, hasThread bool) string {
	if hasThread {
		return fmt.Sprintf(
			"@post('%s', {contentType: 'form'})",
			thoughtsThreadChatAction(threadID, "attach-doc"),
		)
	}
	return fmt.Sprintf(
		"@post('/thoughts/chat/%s/attach-doc', {contentType: 'form'})",
		url.PathEscape(workspaceID),
	)
}

func embeddedWorkspaceStreamURL(
	workspaceID, threadID, runID, docPath string,
	cursor int64,
) string {
	values := url.Values{}
	if runID != "" {
		values.Set("run", runID)
	}
	if docPath != "" {
		values.Set("doc", docPath)
	}
	values.Set("since", strconv.FormatInt(cursor, 10))
	if threadID != "" {
		return thoughtsThreadChatAction(threadID, "stream") + "?" + values.Encode()
	}
	return "/thoughts/chat/" + url.PathEscape(workspaceID) + "/stream?" + values.Encode()
}

func slashCommandInputHandler(workspaceID, threadID, endpointBase string) string {
	threadID = strings.TrimSpace(threadID)
	workspaceID = strings.TrimSpace(workspaceID)
	endpointBase = strings.TrimRight(strings.TrimSpace(endpointBase), "/")
	if endpointBase == "" {
		endpointBase = "/agent-chat"
	}
	commandURL := ""
	switch {
	case threadID != "":
		commandURL = endpointBase + "/thread/" + url.PathEscape(threadID) + "/slash-commands"
	case workspaceID != "":
		commandURL = endpointBase + "/" + url.PathEscape(workspaceID) + "/slash-commands"
	default:
		return "const v = el.value; $slashOpen = v.startsWith('/'); $slashPrefix = $slashOpen ? v.split(/\\s+/)[0] : '';"
	}
	return fmt.Sprintf(`const v = el.value;
const popover = document.getElementById('agent-chat-slash-popover');
const atStart = v.startsWith('/');
$slashOpen = atStart;
$slashPrefix = atStart ? v.split(/\s+/)[0] : '';
if (!popover) return;
if (!atStart) { popover.innerHTML = ''; return; }
const showSlashMessage = (text, className) => {
  const message = document.createElement('div');
  message.className = className;
  message.textContent = text;
  popover.replaceChildren(message);
};
fetch('%s?prefix=' + encodeURIComponent($slashPrefix), {headers: {'Accept': 'application/json'}})
  .then(r => r.ok ? r.json() : [])
  .then(commands => {
    if (!$slashOpen || !Array.isArray(commands) || commands.length === 0) {
      showSlashMessage('No matching Pi commands', 'px-2 py-1 text-xs text-muted-foreground');
      return;
    }
    const buttons = commands.slice(0, 12).map(command => {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'block w-full rounded-md px-2 py-1.5 text-left hover:bg-accent';
      button.addEventListener('click', () => {
        const prompt = document.getElementById('agent-chat-composer-input');
        if (prompt) {
          prompt.value = String(command.name || '') + ' ';
          prompt.focus();
        }
        popover.replaceChildren();
      });

      const name = document.createElement('span');
      name.className = 'text-sm font-medium text-foreground';
      name.textContent = String(command.name || '');
      button.appendChild(name);

      if (command.argument_hint) {
        const hint = document.createElement('span');
        hint.className = 'ml-2 text-xs text-muted-foreground';
        hint.textContent = String(command.argument_hint);
        button.appendChild(hint);
      }

      const source = document.createElement('span');
      source.className = 'ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase text-muted-foreground';
      source.textContent = String(command.source || '');
      button.appendChild(source);

      if (command.description) {
        const description = document.createElement('span');
        description.className = 'block truncate text-xs text-muted-foreground';
        description.textContent = String(command.description);
        button.appendChild(description);
      }
      return button;
    });
    popover.replaceChildren(...buttons);
  })
  .catch(() => { showSlashMessage('Command discovery failed', 'px-2 py-1 text-xs text-destructive'); });`, commandURL)
}

func chatShellSignals(hasSelectedThread bool) string {
	return agentChatShellSignals(hasSelectedThread, agentChatShellSignalOptions{})
}

func chatShellSignalsForPlanSidebar(
	hasSelectedThread bool,
	sidebar PlanSidebarState,
) string {
	return agentChatShellSignals(hasSelectedThread, agentChatShellSignalOptions{
		SidebarOpenGroup:  planSidebarInitialOpenGroup(sidebar),
		SidebarOpenGroups: planSidebarInitialOpenGroups(sidebar),
	})
}

func workspaceShellSignals(hasSelectedThread bool, sidebar PlanSidebarState) string {
	return agentChatShellSignals(hasSelectedThread, agentChatShellSignalOptions{
		IncludeRightRailTab: true,
		SidebarOpenGroup:    planSidebarInitialOpenGroup(sidebar),
		SidebarOpenGroups:   planSidebarInitialOpenGroups(sidebar),
	})
}

func agentChatShellSignals(
	hasSelectedThread bool,
	opts agentChatShellSignalOptions,
) string {
	parts := []string{
		"showDetails: true",
		"showSidebar: " + workspaceSidebarVisibilitySignal(hasSelectedThread),
		"showArtifactTree: false",
		"mobilePane: " + strconv.Quote(agentChatMobilePaneChat),
		"agentChatToast: ''",
	}
	parts = append(
		parts,
		"agentChatSidebarOpenGroup: "+strconv.Quote(opts.SidebarOpenGroup),
		"agentChatSidebarOpenGroups: "+strconv.Quote(opts.SidebarOpenGroups),
	)
	if opts.IncludeRightRailTab {
		parts = append(parts, "rightRailTab: 'artifacts'")
	}
	parts = append(parts, agentChatThreadSheetSignals())
	return "{" + strings.Join(parts, ", ") + "}"
}

func agentChatThreadSheetSignals() string {
	signals := utils.Signals(agentChatThreadSheetID, sheet.SheetSignals{
		Open:        false,
		Modal:       true,
		ReturnValue: nil,
	})
	return strings.TrimSuffix(strings.TrimPrefix(signals.DataSignals, "{"), "}")
}

func agentChatSidebarInitialClass(hasSelectedThread bool) string {
	if agentworkspace.SidebarVisibleByDefault(hasSelectedThread) {
		return "opacity-100 translate-x-0"
	}
	return "opacity-0 -translate-x-4 pointer-events-none"
}

func agentChatDesktopGridInitialClass(hasSelectedThread bool) string {
	if agentworkspace.SidebarVisibleByDefault(hasSelectedThread) {
		return "grid-cols-[minmax(0,1fr)_20rem]"
	}
	return "grid-cols-[minmax(0,1fr)_minmax(0,1fr)]"
}

func agentChatGridInitialClass(hasSelectedThread bool) string {
	return agentChatDesktopGridInitialClass(hasSelectedThread)
}

func workspaceSidebarVisibilitySignal(hasSelectedThread bool) string {
	if agentworkspace.SidebarVisibleByDefault(hasSelectedThread) {
		return "true"
	}
	return "false"
}

func workspaceSidebarInitialOpenGroup(state WorkspaceSidebarState) string {
	return strings.TrimSpace(state.ActiveGroupKey)
}

func planSidebarInitialOpenGroup(state PlanSidebarState) string {
	keys := expandedPlanSidebarNodeKeys(state.Nodes)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func planSidebarInitialOpenGroups(state PlanSidebarState) string {
	keys := expandedPlanSidebarNodeKeys(state.Nodes)
	if len(keys) == 0 {
		return ""
	}
	return "|" + strings.Join(keys, "|") + "|"
}

func expandedPlanSidebarNodeKeys(nodes []PlanSidebarNode) []string {
	keys := []string{}
	for _, node := range nodes {
		if node.Expanded || node.Active {
			if key := strings.TrimSpace(node.Key); key != "" {
				keys = append(keys, key)
			}
		}
		keys = append(keys, expandedPlanSidebarNodeKeys(node.Children)...)
	}
	return keys
}

func workspaceSidebarGroupSignalKey(group ThreadSidebarGroup) string {
	return strings.TrimSpace(group.Key)
}

func workspaceSidebarGroupOpenExpression(group ThreadSidebarGroup) string {
	return "$agentChatSidebarOpenGroup === " + strconv.Quote(
		workspaceSidebarGroupSignalKey(group),
	)
}

func workspaceSidebarGroupSetExpression(group ThreadSidebarGroup) string {
	key := strconv.Quote(workspaceSidebarGroupSignalKey(group))
	return "$agentChatSidebarOpenGroup = ($agentChatSidebarOpenGroup === " + key + " ? \"\" : " + key + ")"
}

func workspacePromptPlaceholder(hasThread bool) string {
	if hasThread {
		return "Continue this conversation"
	}
	return "Ask Pi to do work in this workspace"
}
