package agentchat

import (
	"path/filepath"
	"strings"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/server/services/markdown"
)

func init() {
	markdown.SetEmptyScopeComposer(EmptyScopeComposer)
}

func EmptyScopeComposer(action, attachedDoc string) templ.Component {
	var extra []ComposerHiddenField
	if doc := strings.TrimSpace(attachedDoc); doc != "" {
		extra = append(extra, ComposerHiddenField{Name: "artifact", Value: doc})
	}
	return emptyScopeAgentChatComposer(
		action,
		emptyScopeModeLabel(action, attachedDoc),
		attachFromDoc(attachedDoc),
		extra,
	)
}

func emptyScopeModeLabel(action, attachedDoc string) string {
	switch {
	case strings.Contains(action, "/rooms/dm/"):
		return "agent"
	case strings.Contains(action, "/rooms/freeform"):
		return "freeform"
	case strings.Contains(action, "/rooms/plan/"):
		if emptyScopeIsDocsDesk(action, attachedDoc) {
			return "docs"
		}
		return "plan"
	default:
		return "freeform"
	}
}

func emptyScopeIsDocsDesk(action, attachedDoc string) bool {
	doc := strings.ToLower(filepath.ToSlash(strings.TrimSpace(attachedDoc)))
	doc = strings.TrimPrefix(doc, "thoughts/")
	if strings.HasPrefix(doc, "docs/") || strings.Contains(doc, "/docs/") {
		return true
	}
	const prefix = "/rooms/plan/"
	i := strings.Index(action, prefix)
	if i < 0 {
		return false
	}
	id := action[i+len(prefix):]
	if j := strings.IndexAny(id, "/'"); j >= 0 {
		id = id[:j]
	}
	return strings.HasPrefix(id, "docs--")
}

func attachFromDoc(doc string) []AttachedPath {
	canonical := strings.TrimSpace(doc)
	if canonical == "" {
		return nil
	}
	if parsed := markdown.CanonicalThoughtsDocPathLoose(canonical); parsed != "" {
		canonical = parsed
	}
	canonical = strings.Trim(canonical, "/")
	if canonical == "" {
		return nil
	}
	path := canonical
	if !strings.HasPrefix(path, "thoughts/") {
		path = "thoughts/" + path
	}
	base := filepath.Base(canonical)
	if base == "." || base == "/" {
		return nil
	}
	return []AttachedPath{{Path: path, Basename: base}}
}
