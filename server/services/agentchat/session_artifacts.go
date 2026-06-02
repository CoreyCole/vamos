package agentchat

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	"github.com/CoreyCole/vamos/pkg/db"
)

const defaultAgentSessionAgent = "pi"

type AgentSessionIdentityKind string

const (
	AgentSessionIdentityKindPlanOwned AgentSessionIdentityKind = "plan_owned"
	AgentSessionIdentityKindGlobalPi  AgentSessionIdentityKind = "global_pi"
	AgentSessionIdentityKindWeb       AgentSessionIdentityKind = "web"
)

type SessionPathIdentity struct {
	Kind         AgentSessionIdentityKind
	IdentityPath string
	ResolvedPath string
	PlanOwned    bool
}

type SessionArtifactIndex struct {
	PlanDir                string
	ParentPlanDir          string
	SourceReviewDir        string
	Agent                  string
	Path                   string
	ResolvedPath           string
	SessionID              string
	CWD                    string
	WorkflowID             string
	NodeID                 string
	ContinuedFromSessionID string
	ForkedFromSessionID    string
	Size                   int64
	MTime                  time.Time
	Hash                   string
	LastOffset             int64
	NeedsHydration         bool
}

type AgentSessionMetadata struct {
	SessionID              string `json:"session_id,omitempty"`
	CWD                    string `json:"cwd,omitempty"`
	WorkflowID             string `json:"workflow_id,omitempty"`
	NodeID                 string `json:"node_id,omitempty"`
	ContinuedFromSessionID string `json:"continued_from_session_id,omitempty"`
	ForkedFromSessionID    string `json:"forked_from_session_id,omitempty"`
}

func PlanAgentSessionDir(planDir string, agent string) (string, error) {
	planDir = strings.TrimSpace(planDir)
	agent = strings.TrimSpace(agent)
	if agent == "" {
		agent = defaultAgentSessionAgent
	}
	if planDir == "" {
		return "", errors.New("plan dir is required")
	}
	if agent == "." || agent == ".." || strings.ContainsAny(agent, `/\\`) {
		return "", fmt.Errorf("invalid agent %q", agent)
	}
	return filepath.Join(planDir, ".sessions", agent), nil
}

func ConfigureWorkspaceAgentSessionDir(workspaceDir string, planDir string, agent string) error {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return errors.New("workspace dir is required")
	}
	sessionDir, err := PlanAgentSessionDir(planDir, agent)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return err
	}
	settingsDir := filepath.Join(workspaceDir, ".pi")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(map[string]string{"sessionDir": sessionDir}, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(filepath.Join(settingsDir, "settings.json"), payload, 0o644)
}

func DiscoverPlanAgentSessions(planDir string) ([]SessionArtifactIndex, error) {
	planDir = strings.TrimSpace(planDir)
	if planDir == "" {
		return nil, errors.New("plan dir is required")
	}
	planDir = filepath.Clean(planDir)
	return DiscoverPlanAgentSessionsUnderThoughts(filepath.Dir(filepath.Dir(filepath.Dir(planDir))), planDir)
}

func DiscoverPlanAgentSessionsUnderThoughts(thoughtsRoot, planDir string) ([]SessionArtifactIndex, error) {
	logicalRoot := filepath.Clean(strings.TrimSpace(thoughtsRoot))
	logicalPlanDir := filepath.Clean(strings.TrimSpace(planDir))
	if logicalRoot == "" || logicalRoot == "." {
		return nil, errors.New("thoughts root is required")
	}
	if logicalPlanDir == "" || logicalPlanDir == "." {
		return nil, errors.New("plan dir is required")
	}
	if abs, err := filepath.Abs(logicalRoot); err == nil {
		logicalRoot = abs
	}
	if abs, err := filepath.Abs(logicalPlanDir); err == nil {
		logicalPlanDir = abs
	}
	resolvedRoot, err := filepath.EvalSymlinks(logicalRoot)
	if err != nil {
		return nil, err
	}
	resolvedPlanDir, err := filepath.EvalSymlinks(logicalPlanDir)
	if err != nil {
		return nil, err
	}
	if !pathWithinRoot(resolvedPlanDir, resolvedRoot) {
		return nil, fmt.Errorf("plan dir %q escapes thoughts root %q", logicalPlanDir, logicalRoot)
	}

	var items []SessionArtifactIndex
	err = filepath.WalkDir(logicalPlanDir, func(logicalPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(logicalPath) != jsonlExtension || !pathInPlanSessionDir(logicalPlanDir, logicalPath) {
			return nil
		}
		resolvedPath, err := filepath.EvalSymlinks(logicalPath)
		if err != nil {
			return err
		}
		if !pathWithinRoot(resolvedPath, resolvedRoot) {
			return fmt.Errorf("session path %q escapes thoughts root %q", resolvedPath, resolvedRoot)
		}
		item, err := buildSessionArtifactIndex(logicalRoot, resolvedRoot, logicalPath, resolvedPath)
		if err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})
	return items, err
}

func pathInPlanSessionDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == ".sessions" && i+2 < len(parts) {
			return true
		}
	}
	return false
}

func buildSessionArtifactIndex(logicalRoot, resolvedRoot, logicalPath, resolvedPath string) (SessionArtifactIndex, error) {
	if !pathWithinRoot(resolvedPath, resolvedRoot) {
		return SessionArtifactIndex{}, fmt.Errorf("session path %q escapes thoughts root %q", resolvedPath, resolvedRoot)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	metadata, err := ShallowParseAgentSession(resolvedPath)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	agent, ownerPlanDir, parentPlanDir, sourceReviewDir, err := sessionArtifactOwnership(logicalRoot, logicalPath)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	hash, err := fileSHA256(resolvedPath)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	sessionPath, err := thoughtsRelativeArtifactPath(logicalRoot, logicalPath)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	return SessionArtifactIndex{
		PlanDir:                ownerPlanDir,
		ParentPlanDir:          parentPlanDir,
		SourceReviewDir:        sourceReviewDir,
		Agent:                  agent,
		Path:                   sessionPath,
		ResolvedPath:           resolvedPath,
		SessionID:              metadata.SessionID,
		CWD:                    metadata.CWD,
		WorkflowID:             metadata.WorkflowID,
		NodeID:                 metadata.NodeID,
		ContinuedFromSessionID: metadata.ContinuedFromSessionID,
		ForkedFromSessionID:    metadata.ForkedFromSessionID,
		Size:                   info.Size(),
		MTime:                  info.ModTime(),
		Hash:                   hash,
		LastOffset:             info.Size(),
		NeedsHydration:         true,
	}, nil
}

func sessionArtifactOwnership(logicalRoot, logicalPath string) (agent, ownerPlanDir, parentPlanDir, sourceReviewDir string, err error) {
	rel, err := filepath.Rel(filepath.Clean(logicalRoot), filepath.Clean(logicalPath))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", "", "", "", fmt.Errorf("session path %q is outside thoughts root %q", logicalPath, logicalRoot)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		if part != ".sessions" || i+2 >= len(parts) {
			continue
		}
		agent = parts[i+1]
		ownerPlanDir = path.Join(parts[:i]...)
		if i >= 5 && parts[i-2] == "reviews" {
			parentPlanDir = path.Join(parts[:i-2]...)
			sourceReviewDir = path.Join(parts[:i]...)
		}
		return agent, ownerPlanDir, parentPlanDir, sourceReviewDir, nil
	}
	return "", "", "", "", fmt.Errorf("session path %q is not under .sessions/<agent>", logicalPath)
}

func thoughtsRelativeArtifactPath(logicalRoot, logicalPath string) (string, error) {
	rel, err := filepath.Rel(filepath.Clean(logicalRoot), filepath.Clean(logicalPath))
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("path %q is outside thoughts root %q", logicalPath, logicalRoot)
	}
	return filepath.ToSlash(rel), nil
}

func (s *Service) resolveSessionPathIdentity(input string) (SessionPathIdentity, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return SessionPathIdentity{}, errors.New("session path is required")
	}
	if rel, ok := s.thoughtsRelativePath(input); ok && planOwnedSessionRelPath(rel) {
		resolved := filepath.Join(s.thoughtsRoot, filepath.FromSlash(rel))
		resolved, err := resolveWorkspacePath(resolved)
		if err != nil {
			return SessionPathIdentity{}, err
		}
		thoughtsRoot, err := resolveWorkspacePath(s.thoughtsRoot)
		if err != nil {
			return SessionPathIdentity{}, err
		}
		if !pathWithinRoot(resolved, thoughtsRoot) || !pathInPlanSessionDir(thoughtsRoot, resolved) {
			return SessionPathIdentity{}, fmt.Errorf("session path %q escapes thoughts plan sessions", input)
		}
		return SessionPathIdentity{Kind: AgentSessionIdentityKindPlanOwned, IdentityPath: rel, ResolvedPath: resolved, PlanOwned: true}, nil
	}
	if !filepath.IsAbs(input) && planOwnedSessionRelPath(filepath.ToSlash(input)) {
		identityPath := strings.Trim(strings.TrimSpace(filepath.ToSlash(input)), "/")
		candidate := filepath.Join(s.thoughtsRoot, filepath.FromSlash(identityPath))
		resolved, err := resolveWorkspacePath(candidate)
		if err != nil {
			return SessionPathIdentity{}, err
		}
		thoughtsRoot, err := resolveWorkspacePath(s.thoughtsRoot)
		if err != nil {
			return SessionPathIdentity{}, err
		}
		if !pathWithinRoot(resolved, thoughtsRoot) || !pathInPlanSessionDir(thoughtsRoot, resolved) {
			return SessionPathIdentity{}, fmt.Errorf("session path %q escapes thoughts plan sessions", input)
		}
		return SessionPathIdentity{Kind: AgentSessionIdentityKindPlanOwned, IdentityPath: identityPath, ResolvedPath: resolved, PlanOwned: true}, nil
	}
	resolved, err := s.validatePiSessionPath(input)
	if err != nil {
		return SessionPathIdentity{}, err
	}
	return SessionPathIdentity{Kind: AgentSessionIdentityKindGlobalPi, IdentityPath: resolved, ResolvedPath: resolved}, nil
}

func planOwnedSessionRelPath(rel string) bool {
	parts := strings.Split(strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/"), "/")
	for i, part := range parts {
		if part == ".sessions" && i+2 < len(parts) && strings.HasSuffix(parts[len(parts)-1], jsonlExtension) {
			return true
		}
	}
	return false
}

func (s *Service) HydrateSessionArtifact(ctx context.Context, path string) (chatsession.ChatProjection, error) {
	identity, err := s.resolveSessionPathIdentity(path)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	artifact, err := s.queries.GetAgentSessionByPath(ctx, nullableString(identity.IdentityPath))
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	if artifact.ProjectionState == "needs_hydration" {
		userEmail := strings.TrimSpace(artifact.IndexedByUserEmail.String)
		if userEmail == "" && artifact.IdentityKind == string(AgentSessionIdentityKindPlanOwned) {
			userEmail = "plan-owned-session"
		}
		if _, err := s.ImportPiSession(ctx, SessionImportInput{
			SessionPath: identity.IdentityPath,
			Source:      AgentSessionSourceTerminal,
			UserEmail:   userEmail,
		}); err != nil {
			return chatsession.ChatProjection{}, err
		}
		if err := s.queries.MarkAgentSessionHydratedByPath(ctx, nullableString(identity.IdentityPath)); err != nil {
			return chatsession.ChatProjection{}, err
		}
		artifact, err = s.queries.GetAgentSessionByPath(ctx, nullableString(identity.IdentityPath))
		if err != nil {
			return chatsession.ChatProjection{}, err
		}
	}
	if artifact.ProjectedThreadID.Valid && strings.TrimSpace(artifact.ProjectedThreadID.String) != "" {
		threadID := strings.TrimSpace(artifact.ProjectedThreadID.String)
		if artifact.IdentityKind == string(AgentSessionIdentityKindPlanOwned) {
			return s.sharedChatProjectionFromAgentThread(ctx, threadID)
		}
		return s.chatProjectionFromAgentThread(ctx, threadID)
	}
	return chatsession.ChatProjection{SessionID: strings.TrimSpace(artifact.ExternalSessionID.String)}, nil
}

func (s *Service) sharedChatProjectionFromAgentThread(ctx context.Context, threadID string) (chatsession.ChatProjection, error) {
	thread, err := s.queries.GetSharedAgentThread(ctx, threadID)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	return s.chatProjectionFromThreadRow(ctx, thread)
}

func (s *Service) chatProjectionFromAgentThread(ctx context.Context, threadID string) (chatsession.ChatProjection, error) {
	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	return s.chatProjectionFromThreadRow(ctx, thread)
}

func (s *Service) chatProjectionFromThreadRow(ctx context.Context, thread db.AgentThread) (chatsession.ChatProjection, error) {
	if !thread.HeadEntryID.Valid || strings.TrimSpace(thread.HeadEntryID.String) == "" {
		return chatsession.ChatProjection{SessionID: thread.ID}, nil
	}
	entries, err := s.queries.ListAgentEntryPath(ctx, db.ListAgentEntryPathParams{
		LineageID:   thread.LineageID,
		HeadEntryID: thread.HeadEntryID.String,
	})
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	projection := chatsession.ChatProjection{SessionID: thread.ID}
	for _, entry := range entries {
		message, ok := projectedMessageFromAgentEntry(entry)
		if !ok {
			continue
		}
		projection.Messages = append(projection.Messages, message)
		projection.LastSeq++
	}
	return projection, nil
}

func projectedMessageFromAgentEntry(entry db.ListAgentEntryPathRow) (chatsession.ProjectedMessage, bool) {
	var payload struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(entry.PayloadJson), &payload); err != nil {
		return chatsession.ProjectedMessage{}, false
	}
	if strings.TrimSpace(payload.Type) != "message" {
		return chatsession.ProjectedMessage{}, false
	}
	content := extractContentText(payload.Message.Content)
	role := strings.TrimSpace(payload.Message.Role)
	if role == "" || strings.TrimSpace(content) == "" {
		return chatsession.ProjectedMessage{}, false
	}
	return chatsession.ProjectedMessage{
		ID:      entry.EntryID,
		Role:    role,
		Content: content,
	}, true
}

func ShallowParseAgentSession(path string) (AgentSessionMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return AgentSessionMetadata{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return AgentSessionMetadata{}, err
		}
		return AgentSessionMetadata{}, errors.New("empty session file")
	}
	var header PiSessionHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return AgentSessionMetadata{}, err
	}
	metadata := AgentSessionMetadata{
		SessionID:              strings.TrimSpace(header.ID),
		CWD:                    strings.TrimSpace(header.Cwd),
		ContinuedFromSessionID: strings.TrimSpace(header.ParentSession),
	}
	var raw map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &raw); err == nil {
		metadata.WorkflowID = firstString(raw, "workflow_id", "workflowID", "workflowId")
		metadata.NodeID = firstString(raw, "workflow_node_id", "workflowNodeID", "workflowNodeId", "node_id", "nodeID", "nodeId")
		if value := firstString(raw, "continued_from_session_id", "continuedFromSessionID", "continuedFromSessionId"); value != "" {
			metadata.ContinuedFromSessionID = value
		}
		metadata.ForkedFromSessionID = firstString(raw, "forked_from_session_id", "forkedFromSessionID", "forkedFromSessionId")
	}
	return metadata, nil
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
