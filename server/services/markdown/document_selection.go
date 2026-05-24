package markdown

import (
	"errors"
	"net/url"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
)

type DocumentSelection struct {
	DocPath              string
	Hash                 string
	CommentID            string
	PreserveChat         bool
	PreserveQRSPIContext bool
}

type DirectorySelection struct {
	DirPath string
}

type DocumentEmbeddedChatSelection struct {
	WorkspaceID string
	ThreadID    string
	RunID       string
}

func (state EmbeddedChatLinkState) Selection() DocumentEmbeddedChatSelection {
	return DocumentEmbeddedChatSelection{
		WorkspaceID: strings.TrimSpace(state.WorkspaceID),
		ThreadID:    strings.TrimSpace(state.ThreadID),
		RunID:       strings.TrimSpace(state.RunID),
	}
}

func (state EmbeddedChatLinkState) Preserve(href string) string {
	if !state.Active {
		return href
	}
	selection := state.Selection()
	if selection.WorkspaceID != "" || selection.ThreadID != "" || selection.RunID != "" {
		return PreserveEmbeddedChatQuery(href, selection)
	}
	return preserveChatContextQuery(href)
}

func preserveChatContextQuery(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("context", "chat")
	u.RawQuery = q.Encode()
	return u.String()
}

func EmbeddedChatLinkStateFromRequest(
	c echo.Context,
	fallback EmbeddedChatLinkState,
) EmbeddedChatLinkState {
	state := fallback
	if strings.TrimSpace(c.QueryParam("context")) == thoughtsContextModeChat {
		state.Active = true
	}
	if v := strings.TrimSpace(c.QueryParam("chat_workspace")); v != "" {
		state.WorkspaceID = v
		state.Active = true
	}
	if v := strings.TrimSpace(c.QueryParam("thread")); v != "" {
		state.ThreadID = v
		state.Active = true
	}
	if v := strings.TrimSpace(c.QueryParam("run")); v != "" {
		state.RunID = v
		state.Active = true
	}
	return state
}

func ThoughtsHrefWithChat(rawPath string, isDir bool, chat EmbeddedChatLinkState) string {
	if isDir {
		return chat.Preserve(ThoughtsDirURL(rawPath))
	}
	return chat.Preserve(ThoughtsDocURL(rawPath, ""))
}

func ThoughtsDirURLWithChatState(
	dirPath string,
	selection DocumentEmbeddedChatSelection,
) string {
	return PreserveEmbeddedChatQuery(ThoughtsDirURL(dirPath), selection)
}

func CanonicalThoughtsDocPath(raw string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	trimmed = strings.TrimPrefix(trimmed, "thoughts/")
	if trimmed == "" {
		return "", errors.New("doc_path is required")
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "/thoughts/") {
		return "", errors.New("absolute filesystem paths are not allowed")
	}
	if trimmed == ".." || strings.HasPrefix(trimmed, "../") ||
		strings.Contains(trimmed, "/../") {
		return "", errors.New("doc_path escapes thoughts root")
	}
	clean := path.Clean("/" + trimmed)
	if clean == "/" || clean == "/." {
		return "", errors.New("doc_path is required")
	}
	clean = strings.TrimPrefix(clean, "/")
	if clean == ".." || strings.HasPrefix(clean, "../") ||
		strings.Contains(clean, "/../") {
		return "", errors.New("doc_path escapes thoughts root")
	}
	return clean, nil
}

func CanonicalThoughtsDocPathLoose(raw string) string {
	path, err := CanonicalThoughtsDocPath(raw)
	if err != nil {
		return NormalizeWorkspaceDocPath(raw)
	}
	return path
}

func CanonicalThoughtsDirPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	trimmed := strings.Trim(raw, "/")
	if trimmed == "thoughts" {
		return "", nil
	}
	trimmed = strings.TrimPrefix(trimmed, "thoughts/")
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "/thoughts/") {
		return "", errors.New("absolute filesystem paths are not allowed")
	}
	if trimmed == "" || trimmed == "." {
		return "", nil
	}
	if trimmed == ".." || strings.HasPrefix(trimmed, "../") ||
		strings.Contains(trimmed, "/../") {
		return "", errors.New("dir_path escapes thoughts root")
	}
	clean := path.Clean("/" + trimmed)
	if clean == "/" || clean == "/." {
		return "", nil
	}
	clean = strings.TrimPrefix(clean, "/")
	if clean == ".." || strings.HasPrefix(clean, "../") ||
		strings.Contains(clean, "/../") {
		return "", errors.New("dir_path escapes thoughts root")
	}
	return clean, nil
}

func CanonicalThoughtsDirPathLoose(raw string) string {
	dirPath, err := CanonicalThoughtsDirPath(raw)
	if err != nil {
		return NormalizeWorkspaceDocPath(raw)
	}
	return dirPath
}

func ThoughtsDocURL(docPath, hash string) string {
	canonical := CanonicalThoughtsDocPathLoose(docPath)
	if canonical == "" {
		return "/thoughts/"
	}
	url := thoughtsHref(canonical)
	if hash = strings.TrimSpace(strings.TrimPrefix(hash, "#")); hash != "" {
		url += "#" + hash
	}
	return url
}

func PreserveEmbeddedChatQuery(
	base string,
	selection DocumentEmbeddedChatSelection,
) string {
	if strings.TrimSpace(selection.WorkspaceID) == "" &&
		strings.TrimSpace(selection.ThreadID) == "" &&
		strings.TrimSpace(selection.RunID) == "" {
		return base
	}
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	q.Set("context", "chat")
	if workspaceID := strings.TrimSpace(selection.WorkspaceID); workspaceID != "" {
		q.Set("chat_workspace", workspaceID)
	}
	if threadID := strings.TrimSpace(selection.ThreadID); threadID != "" {
		q.Set("thread", threadID)
	}
	if runID := strings.TrimSpace(selection.RunID); runID != "" {
		q.Set("run", runID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func ThoughtsDocURLWithChatState(
	docPath, hash string,
	selection DocumentEmbeddedChatSelection,
) string {
	return PreserveEmbeddedChatQuery(ThoughtsDocURL(docPath, hash), selection)
}

func ThoughtsDirURL(dirPath string) string {
	canonical := CanonicalThoughtsDirPathLoose(dirPath)
	if canonical == "" {
		return "/thoughts/"
	}
	return thoughtsHref(canonical)
}
