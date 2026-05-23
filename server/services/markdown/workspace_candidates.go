package markdown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type ChatWorkspaceCandidate struct {
	RootPath     string
	AbsoluteRoot string
	Label        string
	WorkspaceID  string
	HasWorkspace bool
}

type OpenChatWorkspaceResult struct {
	WorkspaceID string
	URL         string
}

type ChatWorkspaceOpenInput struct {
	UserEmail    string
	RootDocPath  string
	Title        string
	WorkflowType string
	Source       string
}

type AgentChatWorkspaceOpener interface {
	GetOrCreateWorkspaceForRootDocPath(
		ctx context.Context,
		input ChatWorkspaceOpenInput,
	) (db.Workspace, error)
}

type DBChatWorkspaceCandidateResolver struct {
	queries      *db.Queries
	thoughtsRoot string
	agentChat    AgentChatWorkspaceOpener
}

func NewDBChatWorkspaceCandidateResolver(
	queries *db.Queries,
	thoughtsRoot string,
	opener AgentChatWorkspaceOpener,
) *DBChatWorkspaceCandidateResolver {
	return &DBChatWorkspaceCandidateResolver{
		queries:      queries,
		thoughtsRoot: thoughtsRoot,
		agentChat:    opener,
	}
}

func (r *DBChatWorkspaceCandidateResolver) ResolveChatWorkspaceCandidates(
	ctx context.Context,
	userEmail, documentPath string,
) ([]ChatWorkspaceCandidate, error) {
	if r == nil {
		return nil, nil
	}
	docAbs, err := r.resolveDocPath(documentPath)
	if err != nil {
		return nil, err
	}
	rootAbs, err := resolveExistingOrCleanPath(r.thoughtsRoot)
	if err != nil {
		return nil, err
	}
	if !pathWithinRoot(docAbs, rootAbs) {
		return nil, fmt.Errorf("document path escapes thoughts root")
	}

	existing, err := r.existingWorkspacesByRoot(ctx)
	if err != nil {
		return nil, err
	}

	candidates := make([]ChatWorkspaceCandidate, 0)
	for dir := filepath.Dir(docAbs); ; dir = filepath.Dir(dir) {
		if sameFilesystemPath(dir, rootAbs) || dir == filepath.Dir(dir) {
			break
		}
		if isExcludedCandidate(dir, rootAbs) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			rel, err := thoughtsRelativePath(rootAbs, dir)
			if err != nil {
				return nil, err
			}
			candidate := ChatWorkspaceCandidate{
				RootPath:     rel,
				AbsoluteRoot: dir,
				Label:        candidateLabel(rel),
			}
			if workspace, ok := existing[rel]; ok {
				candidate.HasWorkspace = true
				candidate.WorkspaceID = workspace.ID
			}
			candidates = append(candidates, candidate)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return len(candidates[i].RootPath) < len(candidates[j].RootPath)
	})
	return candidates, nil
}

func (r *DBChatWorkspaceCandidateResolver) OpenChatWorkspace(
	ctx context.Context,
	userEmail, rootPath string,
) (OpenChatWorkspaceResult, error) {
	if r == nil || r.agentChat == nil {
		return OpenChatWorkspaceResult{}, fmt.Errorf(
			"chat workspace resolver is not configured",
		)
	}
	rootPath = filepath.ToSlash(strings.TrimSpace(rootPath))
	if !strings.HasPrefix(rootPath, "thoughts/") {
		return OpenChatWorkspaceResult{}, fmt.Errorf("root path must be under thoughts")
	}

	candidates, err := r.ResolveChatWorkspaceCandidates(
		ctx,
		userEmail,
		rootPath+"/AGENTS.md",
	)
	if err != nil {
		return OpenChatWorkspaceResult{}, err
	}
	var selected ChatWorkspaceCandidate
	for _, candidate := range candidates {
		if candidate.RootPath == rootPath {
			selected = candidate
			break
		}
	}
	if selected.RootPath == "" {
		return OpenChatWorkspaceResult{}, fmt.Errorf(
			"root path is not an AGENTS.md workspace candidate",
		)
	}

	workspace, err := r.agentChat.GetOrCreateWorkspaceForRootDocPath(
		ctx,
		ChatWorkspaceOpenInput{
			UserEmail:    userEmail,
			RootDocPath:  selected.AbsoluteRoot,
			Title:        selected.Label,
			WorkflowType: "freeform",
			Source:       "web",
		},
	)
	if err != nil {
		return OpenChatWorkspaceResult{}, err
	}
	return OpenChatWorkspaceResult{
		WorkspaceID: workspace.ID,
		URL:         "/thoughts/?chat_workspace=" + workspace.ID,
	}, nil
}

func (r *DBChatWorkspaceCandidateResolver) existingWorkspacesByRoot(
	ctx context.Context,
) (map[string]db.Workspace, error) {
	existing := map[string]db.Workspace{}
	if r.queries == nil {
		return existing, nil
	}
	rows, err := r.queries.ListWorkspaces(ctx, 500)
	if err != nil {
		return nil, err
	}
	rootAbs, err := resolveExistingOrCleanPath(r.thoughtsRoot)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		workspaceRoot, err := resolveExistingOrCleanPath(row.RootDocPath)
		if err != nil || !pathWithinRoot(workspaceRoot, rootAbs) {
			continue
		}
		rel, err := thoughtsRelativePath(rootAbs, workspaceRoot)
		if err != nil {
			continue
		}
		existing[rel] = row
	}
	return existing, nil
}

func (r *DBChatWorkspaceCandidateResolver) resolveDocPath(
	documentPath string,
) (string, error) {
	path := strings.TrimSpace(documentPath)
	if path == "" {
		return "", fmt.Errorf("document path is required")
	}
	path = strings.TrimPrefix(path, "thoughts/")
	if filepath.IsAbs(path) {
		return resolveExistingOrCleanPath(path)
	}
	base := strings.TrimSpace(r.thoughtsRoot)
	if base == "" {
		return "", fmt.Errorf("thoughts root is required")
	}
	return resolveExistingOrCleanPath(filepath.Join(base, filepath.FromSlash(path)))
}

func isExcludedCandidate(rootAbs, thoughtsRootAbs string) bool {
	if sameFilesystemPath(rootAbs, thoughtsRootAbs) {
		return true
	}
	rel, err := filepath.Rel(thoughtsRootAbs, rootAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return true
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return len(parts) <= 1
}

func thoughtsRelativePath(thoughtsRootAbs, rootAbs string) (string, error) {
	rel, err := filepath.Rel(thoughtsRootAbs, rootAbs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path is outside thoughts root")
	}
	return "thoughts/" + rel, nil
}

func candidateLabel(rootPath string) string {
	trimmed := strings.Trim(
		strings.TrimPrefix(filepath.ToSlash(rootPath), "thoughts/"),
		"/",
	)
	if trimmed == "" {
		return "Thoughts"
	}
	parts := strings.Split(trimmed, "/")
	base := parts[len(parts)-1]
	if len(parts) >= 3 {
		return base + " · " + strings.Join(parts[:len(parts)-1], "/")
	}
	return base
}

func sameFilesystemPath(a, b string) bool {
	return filepath.Clean(strings.TrimSpace(a)) == filepath.Clean(strings.TrimSpace(b))
}
