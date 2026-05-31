package vamos

import (
	"path"
	"strings"

	"github.com/coreycole/datastarui/e2e/spec"
)

type page struct {
	label string
	path  string
}

func (p page) VisitStep() spec.Step { return spec.Visit(p.path) }
func (p page) Path() string         { return p.path }
func (p page) String() string       { return p.label }

type pages struct{}

var Pages pages

func (pages) Root() page         { return page{label: "root", path: "/"} }
func (pages) Path(p string) page { return page{label: p, path: ensureSlash(p)} }
func (pages) Thoughts() page     { return page{label: "thoughts", path: "/thoughts"} }
func (pages) Thought(p string) page {
	return page{label: "thought " + p, path: "/thoughts/" + strings.TrimPrefix(p, "/")}
}
func (pages) WorkspaceChat() page {
	return page{label: "workspace chat", path: "/agent-chat/workspace"}
}
func (pages) ThoughtChat(docPath, workspaceID, threadID string) page {
	return page{label: "thought chat", path: thoughtsChatURL(docPath, workspaceID, threadID)}
}

func ensureSlash(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + path.Clean(p)
}

// Deprecated string aliases kept until legacy package deletion finishes.
func ThoughtsRootPage() string      { return Pages.Root().Path() }
func ThoughtsWorkbenchPage() string { return Pages.Thoughts().Path() }
func WorkspaceChatPage() string     { return Pages.WorkspaceChat().Path() }
