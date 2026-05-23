package workbench

import (
	"net/url"
	"path"
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

type WorkspaceDocTreeHeaderModel struct {
	RootLabel   string
	CurrentPath string
	Nodes       []WorkspaceDocNode
	EmptyLabel  string
	InitialOpen bool
	TargetID    string
}

type WorkspaceDocNode struct {
	Path       string
	RelPath    string
	Label      string
	Kind       WorkspaceDocKind
	IsActive   bool
	IsExpanded bool
	Children   []WorkspaceDocNode
}

func WorkspaceDocNodeHref(mode DocEntryMode, docPath string) string {
	_ = mode
	trimmed := strings.Trim(strings.TrimSpace(docPath), "/")
	trimmed = strings.TrimPrefix(trimmed, "thoughts/")
	if trimmed == "" {
		return "#"
	}
	parts := strings.Split(path.Clean(trimmed), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return "/thoughts/" + strings.Join(parts, "/")
}

func WorkspaceDocNodeFormAction(mode DocEntryMode, node WorkspaceDocNode) string {
	_ = mode
	if node.Kind == WorkspaceDocKindDir {
		return ""
	}
	return "@post('/thoughts/actions/select-document', {contentType: 'form'})"
}

func WorkspaceDocNodeHiddenFields(node WorkspaceDocNode) map[string]string {
	if node.Kind == WorkspaceDocKindDir {
		return nil
	}
	return map[string]string{"doc_path": node.Path, "qrspi_context_open": "1"}
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
