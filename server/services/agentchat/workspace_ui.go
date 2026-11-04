package agentchat

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	piSessionOpenEndpoint = "/agent-chat/pi-sessions/open"

	agentChatThreadSheetID = "agent_chat_thread_sheet"
)

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
	var commandURL string
	switch {
	case threadID != "":
		commandURL = endpointBase + "/thread/" + url.PathEscape(
			threadID,
		) + "/slash-commands"
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

func workspacePromptPlaceholder(hasThread bool) string {
	if hasThread {
		return "Continue this conversation"
	}
	return "Ask Pi to do work in this workspace"
}

func composerPlaceholderForTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Message…"
	}
	return "Message " + title
}

func sharedThreadComposerPlaceholder(args EmbeddedFreeformPanelArgs) string {
	if placeholder := strings.TrimSpace(args.Placeholder); placeholder != "" {
		return placeholder
	}
	if args.HasThread {
		return "Message…"
	}
	return workspacePromptPlaceholder(false)
}
