package workbench

import (
	"strings"
)

type WorkspaceDocKind string

const (
	WorkspaceDocKindFile WorkspaceDocKind = "file"
	WorkspaceDocKindDir  WorkspaceDocKind = "dir"
)

type WorkspaceDocTreeArgs struct {
	WorkspaceID  string
	RootPath     string
	CurrentPath  string
	Nodes        []WorkspaceDocNode
	EntryMode    DocEntryMode
	EmptyMessage string
}

type WorkspaceDocNode struct {
	Path       string
	RelPath    string
	Label      string
	Kind       WorkspaceDocKind
	Href       string
	IsActive   bool
	IsExpanded bool
	Children   []WorkspaceDocNode
}

func WorkspaceDocNodeToggleSignal(node WorkspaceDocNode) string {
	key := strings.TrimSpace(node.RelPath)
	if key == "" || key == "." {
		key = strings.TrimSpace(node.Path)
	}
	key = strings.Trim(key, "/")
	if key == "" {
		key = "root"
	}

	var b strings.Builder
	b.WriteString("workspaceDocTreeNode_")
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
