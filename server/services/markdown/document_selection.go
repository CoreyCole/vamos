package markdown

import (
	"errors"
	"net/url"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
)

type DocumentSelection struct {
	DocPath      string
	Hash         string
	CommentID    string
	PreserveChat bool
}

type DocumentEmbeddedChatSelection struct {
	WorkspaceID string
	ThreadID    string
	RunID       string
}

func ThoughtsWorkbenchLinkStateFromRequest(
	c echo.Context,
	selectedPlanPath string,
) ThoughtsWorkbenchLinkState {
	return ThoughtsWorkbenchLinkState{
		Context:          thoughtsContextMode(c),
		ChatWorkspaceID:  strings.TrimSpace(c.QueryParam("chat_workspace")),
		ChatThreadID:     strings.TrimSpace(c.QueryParam("thread")),
		ChatRunID:        strings.TrimSpace(c.QueryParam("run")),
		HermesThreadID:   strings.TrimSpace(c.QueryParam("hermes_thread")),
		SelectedPlanPath: normalizeThoughtsRelativePath(selectedPlanPath),
	}
}

func (state ThoughtsWorkbenchLinkState) WithContext(contextMode string) ThoughtsWorkbenchLinkState {
	state.Context = strings.TrimSpace(contextMode)
	return state
}

func (state ThoughtsWorkbenchLinkState) WithHermesThread(threadID string) ThoughtsWorkbenchLinkState {
	state.Context = thoughtsContextModeThreads
	state.HermesThreadID = strings.TrimSpace(threadID)
	return state
}

func (state ThoughtsWorkbenchLinkState) WithoutHermesThread() ThoughtsWorkbenchLinkState {
	state.HermesThreadID = ""
	return state
}

func (state ThoughtsWorkbenchLinkState) HiddenFields() map[string]string {
	fields := map[string]string{}
	for key, value := range map[string]string{
		"context": state.Context, "chat_workspace": state.ChatWorkspaceID,
		"thread": state.ChatThreadID, "run": state.ChatRunID,
		"hermes_thread": state.HermesThreadID,
	} {
		if value = strings.TrimSpace(value); value != "" {
			fields[key] = value
		}
	}
	return fields
}

func (state ThoughtsWorkbenchLinkState) Preserve(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	q := u.Query()
	if contextMode := strings.TrimSpace(state.Context); contextMode != "" {
		q.Set("context", contextMode)
	}
	setQueryValue(q, "chat_workspace", state.ChatWorkspaceID)
	setQueryValue(q, "thread", state.ChatThreadID)
	setQueryValue(q, "run", state.ChatRunID)
	setQueryValue(q, "hermes_thread", state.HermesThreadID)
	u.RawQuery = q.Encode()
	return u.String()
}

func setQueryValue(query url.Values, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		query.Set(key, value)
	} else {
		query.Del(key)
	}
}

func ThoughtsHrefWithWorkbenchState(
	rawPath string,
	isDir bool,
	state ThoughtsWorkbenchLinkState,
) string {
	if isDir {
		return state.Preserve(ThoughtsDirURL(rawPath))
	}
	return state.Preserve(ThoughtsDocURL(rawPath, ""))
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
	state := ThoughtsWorkbenchLinkState{
		Context:         thoughtsContextModeChat,
		ChatWorkspaceID: selection.WorkspaceID,
		ChatThreadID:    selection.ThreadID,
		ChatRunID:       selection.RunID,
	}
	if strings.TrimSpace(selection.ThreadID) != "" {
		state.ChatWorkspaceID = ""
	}
	return state.Preserve(base)
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
