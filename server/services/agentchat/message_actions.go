package agentchat

import (
	"strconv"
	"strings"

	"github.com/CoreyCole/vamos/pkg/datastarui/components/toast"
	"github.com/CoreyCole/vamos/pkg/datastarui/utils"
)

type msgActionsSignals struct {
	Open bool `json:"open"`
}

func msgActionsID(domID string) string {
	return "msg-" + strings.TrimSpace(domID) + "-actions"
}

func msgMenuID(domID string) string {
	return "msg-" + strings.TrimSpace(domID) + "-menu"
}

func msgSheetID(domID string) string {
	return "msg-" + strings.TrimSpace(domID) + "-sheet"
}

func msgActionsSignalsManager(domID string) *utils.SignalManager {
	return utils.Signals(msgMenuID(domID), msgActionsSignals{Open: false})
}

func msgActionsOpenExpr(domID string) string {
	return msgActionsSignalsManager(domID).Signal("open")
}

func msgActionsToggleExpr(domID string) string {
	return msgActionsSignalsManager(domID).Toggle("open")
}

func msgActionsSetOpenExpr(domID string, open bool) string {
	return msgActionsSignalsManager(domID).Set("open", strconv.FormatBool(open))
}

func msgActionsCloseExpr(domID string) string {
	return msgActionsSetOpenExpr(domID, false)
}

// messageCopyClickExpr copies plain text then shows root clipboard_success toast on success only.
func messageCopyClickExpr(domID, content string) string {
	show := toast.ShowToastExpr("clipboard_success", 2000)
	closeMenu := msgActionsCloseExpr(domID)
	return "navigator.clipboard.writeText(" + strconv.Quote(
		content,
	) + ").then(() => { " + show + " }); " + closeMenu
}

// messageQuoteClickExpr prepends a markdown quote into the composer textarea.
// chatDraft is local to #agent-chat-composer-form; writing $chatDraft here would
// miss that bind. Dispatch input so data-bind and autosize see the new value.
func messageQuoteClickExpr(domID, content string) string {
	text := strings.TrimSpace(content)
	quoted := ""
	if text != "" {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = "> " + line
		}
		quoted = strings.Join(lines, "\n") + "\n\n"
	}
	closeMenu := msgActionsCloseExpr(domID)
	return "(() => { const el = document.getElementById('agent-chat-composer-input'); const next = " +
		strconv.Quote(
			quoted,
		) +
		" + ((el && el.value) || ''); if (el) { el.value = next; el.dispatchEvent(new Event('input', { bubbles: true })); el.focus(); } })(); " +
		closeMenu
}

func messagePlainText(msg TranscriptMessage) string {
	if strings.TrimSpace(msg.Content) != "" {
		return msg.Content
	}
	html := msg.HTMLContent
	if html == "" {
		return ""
	}
	var b strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func msgActionsIsUser(msg TranscriptMessage) bool {
	return strings.EqualFold(strings.TrimSpace(msg.Role), "user")
}
