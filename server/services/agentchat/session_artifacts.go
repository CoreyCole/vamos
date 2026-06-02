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
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/agents/chatsession"
	"github.com/CoreyCole/vamos/pkg/db"
)

const defaultAgentSessionAgent = "pi"

type SessionArtifactIndex struct {
	PlanDir                string
	ParentPlanDir          string
	SourceReviewDir        string
	Agent                  string
	Path                   string
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
	root, err := filepath.EvalSymlinks(planDir)
	if err != nil {
		return nil, err
	}
	var items []SessionArtifactIndex
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != jsonlExtension || !pathInPlanSessionDir(root, path) {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		if !pathWithinRoot(resolved, root) {
			return fmt.Errorf("session path %q escapes plan dir %q", resolved, root)
		}
		item, err := buildSessionArtifactIndex(root, resolved)
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

func buildSessionArtifactIndex(planRoot, path string) (SessionArtifactIndex, error) {
	info, err := os.Stat(path)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	metadata, err := ShallowParseAgentSession(path)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	agent, ownerPlanDir, parentPlanDir, sourceReviewDir, err := sessionArtifactOwnership(planRoot, path)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	hash, err := fileSHA256(path)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	return SessionArtifactIndex{
		PlanDir:                ownerPlanDir,
		ParentPlanDir:          parentPlanDir,
		SourceReviewDir:        sourceReviewDir,
		Agent:                  agent,
		Path:                   path,
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

func sessionArtifactOwnership(planRoot, path string) (agent, ownerPlanDir, parentPlanDir, sourceReviewDir string, err error) {
	rel, err := filepath.Rel(planRoot, path)
	if err != nil {
		return "", "", "", "", err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part != ".sessions" || i+2 >= len(parts) {
			continue
		}
		agent = parts[i+1]
		ownerPlanDir = filepath.Join(append([]string{planRoot}, parts[:i]...)...)
		if i >= 2 && parts[0] == "reviews" {
			parentPlanDir = planRoot
			sourceReviewDir = filepath.Join(planRoot, "reviews", parts[1])
		}
		return agent, ownerPlanDir, parentPlanDir, sourceReviewDir, nil
	}
	return "", "", "", "", fmt.Errorf("session path %q is not under .sessions/<agent>", path)
}

func (s *Service) HydrateSessionArtifact(ctx context.Context, path string) (chatsession.ChatProjection, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return chatsession.ChatProjection{}, errors.New("session path is required")
	}
	resolvedPath, err := s.validatePiSessionPath(path)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	artifact, err := s.queries.GetAgentSessionByPath(ctx, nullableString(resolvedPath))
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	if artifact.NeedsHydration != 0 && artifact.Status != "imported" && artifact.Status != "diverged" {
		if _, err := s.ImportPiSession(ctx, SessionImportInput{
			SessionPath: resolvedPath,
			Source:      AgentSessionSourceTerminal,
			UserEmail:   strings.TrimSpace(artifact.UserEmail.String),
		}); err != nil {
			return chatsession.ChatProjection{}, err
		}
		if err := s.queries.MarkAgentSessionHydratedByPath(ctx, nullableString(resolvedPath)); err != nil {
			return chatsession.ChatProjection{}, err
		}
		artifact, err = s.queries.GetAgentSessionByPath(ctx, nullableString(resolvedPath))
		if err != nil {
			return chatsession.ChatProjection{}, err
		}
	}
	if artifact.ThreadID.Valid && strings.TrimSpace(artifact.ThreadID.String) != "" {
		return s.chatProjectionFromAgentThread(ctx, strings.TrimSpace(artifact.ThreadID.String))
	}
	return chatsession.ChatProjection{SessionID: strings.TrimSpace(artifact.SessionID.String)}, nil
}

func (s *Service) chatProjectionFromAgentThread(ctx context.Context, threadID string) (chatsession.ChatProjection, error) {
	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
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
