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
	return emptyScopeAgentChatComposer(action, attachFromDoc(attachedDoc), extra)
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
