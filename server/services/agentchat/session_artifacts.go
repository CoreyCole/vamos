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

	"gopkg.in/yaml.v3"

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
	ResultPath             string
	Checkpoints            []CheckpointArtifact
	HermesMetadata         *HermesTranscriptEvent
}

type CheckpointArtifact struct {
	FinalEntryID string
	Path         string
}

type CheckpointDeliveryIdentity struct {
	ThreadID     string
	SessionID    string
	FinalEntryID string
}

// CheckpointDeliveryProjection is rebuildable server state keyed by immutable
// checkpoint identity. It never changes the checkpoint artifact.
type CheckpointDeliveryProjection struct {
	Identity    CheckpointDeliveryIdentity
	Delivered   bool
	LastAttempt uint64
}

type CheckpointDeliveryProjectionStore interface {
	GetCheckpointDelivery(
		CheckpointDeliveryIdentity,
	) (CheckpointDeliveryProjection, bool, error)
	PutCheckpointDelivery(CheckpointDeliveryProjection) error
}

type HermesThreadArtifact struct {
	ID              string
	PlanDir         string
	PlanIdentity    HermesPlanIdentity
	Path            string
	CreatorEmail    string
	PromptAuthority HermesPromptAuthority
	Title           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type AgentSessionMetadata struct {
	SessionID              string `json:"session_id,omitempty"`
	CWD                    string `json:"cwd,omitempty"`
	WorkflowID             string `json:"workflow_id,omitempty"`
	NodeID                 string `json:"node_id,omitempty"`
	ContinuedFromSessionID string `json:"continued_from_session_id,omitempty"`
	ForkedFromSessionID    string `json:"forked_from_session_id,omitempty"`
}

func PlanAgentSessionDir(planDir, agent string) (string, error) {
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
	return filepath.Join(planDir, ".vamos", "sessions", agent), nil
}

func ConfigureWorkspaceAgentSessionDir(workspaceDir, planDir, agent string) error {
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
	payload, err := json.MarshalIndent(
		map[string]string{"sessionDir": sessionDir},
		"",
		"  ",
	)
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
	return DiscoverPlanAgentSessionsUnderThoughts(
		filepath.Dir(filepath.Dir(filepath.Dir(planDir))),
		planDir,
	)
}

func DiscoverPlanAgentSessionsUnderThoughts(
	thoughtsRoot, planDir string,
) ([]SessionArtifactIndex, error) {
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
		return nil, fmt.Errorf(
			"plan dir %q escapes thoughts root %q",
			logicalPlanDir,
			logicalRoot,
		)
	}

	var items []SessionArtifactIndex
	err = filepath.WalkDir(
		logicalPlanDir,
		func(logicalPath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(logicalPath) != jsonlExtension ||
				!pathInPlanSessionDir(logicalPlanDir, logicalPath) {
				return nil
			}
			resolvedPath, err := filepath.EvalSymlinks(logicalPath)
			if err != nil {
				return err
			}
			if !pathWithinRoot(resolvedPath, resolvedRoot) {
				return fmt.Errorf(
					"session path %q escapes thoughts root %q",
					resolvedPath,
					resolvedRoot,
				)
			}
			item, err := buildSessionArtifactIndex(
				logicalRoot,
				resolvedRoot,
				logicalPath,
				resolvedPath,
			)
			if err != nil {
				return err
			}
			items = append(items, item)
			return nil
		},
	)
	return items, err
}

// ScanPlanSessions discovers canonical Pi and Hermes transcript JSONL files.
// planDir may be any plan directory under thoughtsRoot.
func ScanPlanSessions(thoughtsRoot, planDir string) ([]SessionArtifactIndex, error) {
	return DiscoverPlanAgentSessionsUnderThoughts(thoughtsRoot, planDir)
}

// ScanHermesThreads is a disk fallback for thread discovery before the database
// projection has observed a newly appended transcript.
func ScanHermesThreads(thoughtsRoot, root string) ([]HermesThreadArtifact, error) {
	if strings.TrimSpace(thoughtsRoot) == "" {
		return nil, errors.New("thoughts root is required")
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("plan dir is required")
	}
	logicalRoot, err := filepath.Abs(filepath.Clean(strings.TrimSpace(thoughtsRoot)))
	if err != nil {
		return nil, err
	}
	logicalPlan, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return nil, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(logicalRoot)
	if err != nil {
		return nil, err
	}
	resolvedPlan, err := filepath.EvalSymlinks(logicalPlan)
	if err != nil {
		return nil, err
	}
	if !pathWithinRoot(resolvedPlan, resolvedRoot) {
		return nil, fmt.Errorf("plan dir %q escapes thoughts root %q", logicalPlan, logicalRoot)
	}
	hermesDir := filepath.Join(logicalPlan, ".vamos", "sessions", "hermes")
	entries, err := os.ReadDir(hermesDir)
	if errors.Is(err, os.ErrNotExist) {
		return []HermesThreadArtifact{}, nil
	}
	if err != nil {
		return nil, err
	}
	threads := make([]HermesThreadArtifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != jsonlExtension {
			continue
		}
		logicalPath := filepath.Join(hermesDir, entry.Name())
		resolvedPath, err := filepath.EvalSymlinks(logicalPath)
		if err != nil {
			return nil, err
		}
		if !pathWithinRoot(resolvedPath, resolvedRoot) {
			return nil, fmt.Errorf("session path %q escapes thoughts root %q", resolvedPath, resolvedRoot)
		}
		item, err := buildSessionArtifactIndex(logicalRoot, resolvedRoot, logicalPath, resolvedPath)
		if err != nil {
			return nil, err
		}
		if item.Agent != "hermes" || item.HermesMetadata == nil ||
			item.HermesMetadata.PromptAuthority == nil {
			return nil, fmt.Errorf("Hermes artifact %q lacks metadata", item.Path)
		}
		threads = append(threads, HermesThreadArtifact{
			ID: item.SessionID, PlanDir: item.PlanDir,
			PlanIdentity: item.HermesMetadata.PlanDir, Path: item.Path,
			CreatorEmail:    item.HermesMetadata.CreatorEmail,
			PromptAuthority: *item.HermesMetadata.PromptAuthority,
			Title:           item.HermesMetadata.Title, CreatedAt: item.HermesMetadata.At,
			UpdatedAt: item.MTime,
		})
	}
	return threads, nil
}

func pathInPlanSessionDir(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		if part == ".vamos" && i+3 < len(parts) && parts[i+1] == "sessions" {
			return true
		}
	}
	return false
}

func buildSessionArtifactIndex(
	logicalRoot, resolvedRoot, logicalPath, resolvedPath string,
) (SessionArtifactIndex, error) {
	if !pathWithinRoot(resolvedPath, resolvedRoot) {
		return SessionArtifactIndex{}, fmt.Errorf(
			"session path %q escapes thoughts root %q",
			resolvedPath,
			resolvedRoot,
		)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	agent, ownerPlanDir, parentPlanDir, sourceReviewDir, err := sessionArtifactOwnership(
		logicalRoot,
		logicalPath,
	)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	metadata, hermesMetadata, err := shallowParseSessionArtifact(
		resolvedPath, agent, filepath.Join(logicalRoot, filepath.FromSlash(ownerPlanDir)),
		HermesPlanIdentity(ownerPlanDir),
	)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	var hash string
	if agent == "hermes" {
		threadID := strings.TrimSuffix(filepath.Base(logicalPath), jsonlExtension)
		lock, lockErr := acquireHermesTranscriptLock(
			context.Background(), filepath.Join(logicalRoot, filepath.FromSlash(ownerPlanDir)),
			threadID, true,
		)
		if lockErr != nil {
			return SessionArtifactIndex{}, lockErr
		}
		hash, err = fileSHA256(resolvedPath)
		closeErr := lock.Close()
		if err == nil {
			err = closeErr
		}
	} else {
		hash, err = fileSHA256(resolvedPath)
	}
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	sessionPath, err := thoughtsRelativeArtifactPath(logicalRoot, logicalPath)
	if err != nil {
		return SessionArtifactIndex{}, err
	}
	resultPath := ""
	var checkpoints []CheckpointArtifact
	if agent == "pi" {
		candidate := strings.TrimSuffix(logicalPath, jsonlExtension) + "_result.yaml"
		if _, err := os.Stat(candidate); err == nil {
			resolvedResult, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return SessionArtifactIndex{}, err
			}
			if !pathWithinRoot(resolvedResult, resolvedRoot) {
				return SessionArtifactIndex{}, fmt.Errorf(
					"Pi result path %q escapes thoughts root",
					resolvedResult,
				)
			}
			resultPath, err = thoughtsRelativeArtifactPath(logicalRoot, candidate)
			if err != nil {
				return SessionArtifactIndex{}, err
			}
		}
		checkpoints, err = discoverCheckpointArtifacts(
			logicalRoot, resolvedRoot, logicalPath, metadata.SessionID,
		)
		if err != nil {
			return SessionArtifactIndex{}, err
		}
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
		NeedsHydration:         agent != "hermes",
		ResultPath:             resultPath,
		Checkpoints:            checkpoints,
		HermesMetadata:         hermesMetadata,
	}, nil
}

func discoverCheckpointArtifacts(
	logicalRoot, resolvedRoot, logicalSessionPath, sessionID string,
) ([]CheckpointArtifact, error) {
	if err := validateCheckpointComponent(sessionID); err != nil {
		return nil, fmt.Errorf("checkpoint session: %w", err)
	}
	directory := filepath.Join(
		filepath.Dir(logicalSessionPath), sessionID, "checkpoints",
	)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	artifacts := make([]CheckpointArtifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		finalEntryID := strings.TrimSuffix(entry.Name(), ".yaml")
		if err := validateCheckpointComponent(finalEntryID); err != nil {
			return nil, fmt.Errorf("checkpoint final entry: %w", err)
		}
		logicalPath := filepath.Join(directory, entry.Name())
		data, err := os.ReadFile(logicalPath)
		if err != nil {
			return nil, err
		}
		var header struct {
			Version int `yaml:"version"`
		}
		if err := yaml.Unmarshal(data, &header); err != nil {
			return nil, fmt.Errorf("parse checkpoint %q: %w", logicalPath, err)
		}
		if header.Version != 2 {
			return nil, fmt.Errorf("checkpoint %q is not schema v2", logicalPath)
		}
		resolvedPath, err := filepath.EvalSymlinks(logicalPath)
		if err != nil {
			return nil, err
		}
		if !pathWithinRoot(resolvedPath, resolvedRoot) {
			return nil, fmt.Errorf(
				"checkpoint path %q escapes thoughts root",
				resolvedPath,
			)
		}
		rel, err := thoughtsRelativeArtifactPath(logicalRoot, logicalPath)
		if err != nil {
			return nil, err
		}
		artifacts = append(
			artifacts,
			CheckpointArtifact{FinalEntryID: finalEntryID, Path: rel},
		)
	}
	return artifacts, nil
}

func validateCheckpointComponent(value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("unsafe path component %q", value)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return fmt.Errorf("unsafe path component %q", value)
	}
	return nil
}

func sessionArtifactOwnership(
	logicalRoot, logicalPath string,
) (agent, ownerPlanDir, parentPlanDir, sourceReviewDir string, err error) {
	rel, err := filepath.Rel(filepath.Clean(logicalRoot), filepath.Clean(logicalPath))
	if err != nil || rel == "." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		rel == ".." {
		return "", "", "", "", fmt.Errorf(
			"session path %q is outside thoughts root %q",
			logicalPath,
			logicalRoot,
		)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		if part != ".vamos" || i+3 >= len(parts) || parts[i+1] != "sessions" {
			continue
		}
		agent = parts[i+2]
		ownerPlanDir = path.Join(parts[:i]...)
		if i >= 5 && parts[i-2] == "reviews" {
			parentPlanDir = path.Join(parts[:i-2]...)
			sourceReviewDir = path.Join(parts[:i]...)
		}
		return agent, ownerPlanDir, parentPlanDir, sourceReviewDir, nil
	}
	return "", "", "", "", fmt.Errorf(
		"session path %q is not under .vamos/sessions/<agent>",
		logicalPath,
	)
}

func thoughtsRelativeArtifactPath(logicalRoot, logicalPath string) (string, error) {
	rel, err := filepath.Rel(filepath.Clean(logicalRoot), filepath.Clean(logicalPath))
	if err != nil || rel == "." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		rel == ".." {
		return "", fmt.Errorf(
			"path %q is outside thoughts root %q",
			logicalPath,
			logicalRoot,
		)
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
		if !pathWithinRoot(resolved, thoughtsRoot) ||
			!pathInPlanSessionDir(thoughtsRoot, resolved) {
			return SessionPathIdentity{}, fmt.Errorf(
				"session path %q escapes thoughts plan sessions",
				input,
			)
		}
		return SessionPathIdentity{
			Kind:         AgentSessionIdentityKindPlanOwned,
			IdentityPath: rel,
			ResolvedPath: resolved,
			PlanOwned:    true,
		}, nil
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
		if !pathWithinRoot(resolved, thoughtsRoot) ||
			!pathInPlanSessionDir(thoughtsRoot, resolved) {
			return SessionPathIdentity{}, fmt.Errorf(
				"session path %q escapes thoughts plan sessions",
				input,
			)
		}
		return SessionPathIdentity{
			Kind:         AgentSessionIdentityKindPlanOwned,
			IdentityPath: identityPath,
			ResolvedPath: resolved,
			PlanOwned:    true,
		}, nil
	}
	resolved, err := s.validatePiSessionPath(input)
	if err != nil {
		return SessionPathIdentity{}, err
	}
	return SessionPathIdentity{
		Kind:         AgentSessionIdentityKindGlobalPi,
		IdentityPath: resolved,
		ResolvedPath: resolved,
	}, nil
}

func planOwnedSessionRelPath(rel string) bool {
	parts := strings.Split(
		strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/"),
		"/",
	)
	for i, part := range parts {
		if part == ".vamos" && i+3 < len(parts) && parts[i+1] == "sessions" &&
			strings.HasSuffix(parts[len(parts)-1], jsonlExtension) {
			return true
		}
	}
	return false
}

func (s *Service) HydrateSessionArtifact(
	ctx context.Context,
	path string,
) (chatsession.ChatProjection, error) {
	identity, err := s.resolveSessionPathIdentity(path)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	artifact, err := s.queries.GetAgentSessionByPath(
		ctx,
		nullableString(identity.IdentityPath),
	)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	if artifact.Agent == "hermes" {
		return chatsession.ChatProjection{
			SessionID: strings.TrimSpace(artifact.ExternalSessionID.String),
		}, nil
	}
	if artifact.ProjectionState == "needs_hydration" {
		userEmail := strings.TrimSpace(artifact.IndexedByUserEmail.String)
		if userEmail == "" &&
			artifact.IdentityKind == string(AgentSessionIdentityKindPlanOwned) {
			userEmail = "plan-owned-session"
		}
		if _, err := s.ImportPiSession(ctx, SessionImportInput{
			SessionPath: identity.IdentityPath,
			Source:      AgentSessionSourceTerminal,
			UserEmail:   userEmail,
		}); err != nil {
			return chatsession.ChatProjection{}, err
		}
		if err := s.queries.MarkAgentSessionHydratedByPath(
			ctx,
			nullableString(identity.IdentityPath),
		); err != nil {
			return chatsession.ChatProjection{}, err
		}
		artifact, err = s.queries.GetAgentSessionByPath(
			ctx,
			nullableString(identity.IdentityPath),
		)
		if err != nil {
			return chatsession.ChatProjection{}, err
		}
	}
	if artifact.ProjectedThreadID.Valid &&
		strings.TrimSpace(artifact.ProjectedThreadID.String) != "" {
		threadID := strings.TrimSpace(artifact.ProjectedThreadID.String)
		if artifact.IdentityKind == string(AgentSessionIdentityKindPlanOwned) {
			return s.sharedChatProjectionFromAgentThread(ctx, threadID)
		}
		return s.chatProjectionFromAgentThread(ctx, threadID)
	}
	return chatsession.ChatProjection{
		SessionID: strings.TrimSpace(artifact.ExternalSessionID.String),
	}, nil
}

func (s *Service) sharedChatProjectionFromAgentThread(
	ctx context.Context,
	threadID string,
) (chatsession.ChatProjection, error) {
	thread, err := s.queries.GetSharedAgentThread(ctx, threadID)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	return s.chatProjectionFromThreadRow(ctx, thread)
}

func (s *Service) chatProjectionFromAgentThread(
	ctx context.Context,
	threadID string,
) (chatsession.ChatProjection, error) {
	thread, err := s.queries.GetAgentThread(ctx, threadID)
	if err != nil {
		return chatsession.ChatProjection{}, err
	}
	return s.chatProjectionFromThreadRow(ctx, thread)
}

func (s *Service) chatProjectionFromThreadRow(
	ctx context.Context,
	thread db.AgentThread,
) (chatsession.ChatProjection, error) {
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

func projectedMessageFromAgentEntry(
	entry db.ListAgentEntryPathRow,
) (chatsession.ProjectedMessage, bool) {
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

func shallowParseSessionArtifact(
	path, agent, planDir string, planIdentity HermesPlanIdentity,
) (AgentSessionMetadata, *HermesTranscriptEvent, error) {
	if agent == "hermes" {
		return shallowParseHermesTranscript(path, planDir, planIdentity)
	}
	metadata, err := ShallowParseAgentSession(path)
	return metadata, nil, err
}

func shallowParseHermesTranscript(
	path, planDir string, planIdentity HermesPlanIdentity,
) (AgentSessionMetadata, *HermesTranscriptEvent, error) {
	threadID := strings.TrimSuffix(filepath.Base(path), jsonlExtension)
	events, err := readHermesTranscriptContext(
		context.Background(), planDir, planIdentity, threadID,
	)
	if err != nil {
		return AgentSessionMetadata{}, nil, err
	}
	metadata := events[0]
	return AgentSessionMetadata{SessionID: metadata.ThreadID}, &metadata, nil
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
		metadata.NodeID = firstString(
			raw,
			"workflow_node_id",
			"workflowNodeID",
			"workflowNodeId",
			"node_id",
			"nodeID",
			"nodeId",
		)
		if value := firstString(
			raw,
			"continued_from_session_id",
			"continuedFromSessionID",
			"continuedFromSessionId",
		); value != "" {
			metadata.ContinuedFromSessionID = value
		}
		metadata.ForkedFromSessionID = firstString(
			raw,
			"forked_from_session_id",
			"forkedFromSessionID",
			"forkedFromSessionId",
		)
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
