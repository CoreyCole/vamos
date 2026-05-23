package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/collections"
	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) RequestPiSessionIndex(req PiSessionIndexRequest) {
	user := normalizePlanSidebarUser(req.UserEmail)
	if user == "" {
		return
	}
	req.UserEmail = user

	s.piIndexMu.Lock()
	if s.piIndexRunning[user] {
		s.piIndexQueued[user] = mergePiSessionIndexRequest(s.piIndexQueued[user], req)
		s.piIndexMu.Unlock()
		return
	}
	s.piIndexRunning[user] = true
	s.piIndexMu.Unlock()

	go s.runPiSessionIndexLoop(user, req)
}

func mergePiSessionIndexRequest(
	current PiSessionIndexRequest,
	next PiSessionIndexRequest,
) PiSessionIndexRequest {
	if strings.TrimSpace(current.UserEmail) == "" {
		return next
	}
	current.Force = current.Force || next.Force
	if strings.TrimSpace(next.Reason) != "" {
		if strings.TrimSpace(current.Reason) == "" {
			current.Reason = strings.TrimSpace(next.Reason)
		} else if !strings.Contains(current.Reason, next.Reason) {
			current.Reason += "," + strings.TrimSpace(next.Reason)
		}
	}
	return current
}

const piSessionIndexTimeout = 2 * time.Minute

func (s *Service) runPiSessionIndexLoop(user string, req PiSessionIndexRequest) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), piSessionIndexTimeout)
		result, err := s.indexPiSessions(ctx, req)
		cancel()
		if err == nil && (result.Indexed > 0 || result.Imported > 0) {
			s.NotifyPlanSidebar(user)
			for _, workspaceID := range result.AffectedWorkspaces {
				s.notifier.NotifyWorkspaceResource(workspaceID)
			}
		}

		s.piIndexMu.Lock()
		next, queued := s.piIndexQueued[user]
		delete(s.piIndexQueued, user)
		if !queued {
			delete(s.piIndexRunning, user)
			s.piIndexMu.Unlock()
			return
		}
		req = next
		s.piIndexMu.Unlock()
	}
}

func (s *Service) indexPiSessions(
	ctx context.Context,
	req PiSessionIndexRequest,
) (PiSessionIndexResult, error) {
	user := normalizePlanSidebarUser(req.UserEmail)
	if user == "" {
		return PiSessionIndexResult{}, nil
	}
	root := strings.TrimSpace(s.piSessionsDir)
	if root == "" {
		return PiSessionIndexResult{}, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PiSessionIndexResult{}, nil
		}
		return PiSessionIndexResult{}, err
	}
	if !info.IsDir() {
		return PiSessionIndexResult{}, nil
	}

	result := PiSessionIndexResult{}
	affectedPlans := collections.NewSet[string]()
	affectedWorkspaces := collections.NewSet[string]()

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != jsonlExtension {
			return nil
		}
		record, indexErr := s.indexPiSessionFile(ctx, user, path)
		if indexErr != nil {
			result.Failed++
			return nil //nolint:nilerr // Keep indexing best-effort across corrupt session files.
		}
		result.Indexed++
		if record.InferredPlanDir.Valid && record.InferredPlanDir.String != "" {
			affectedPlans.Add(record.InferredPlanDir.String)
		}
		if record.WorkspaceID.Valid && record.WorkspaceID.String != "" {
			affectedWorkspaces.Add(record.WorkspaceID.String)
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	result.AffectedPlans = sortedSetValues(affectedPlans)
	result.AffectedWorkspaces = sortedSetValues(affectedWorkspaces)
	return result, nil
}

func (s *Service) indexPiSessionFile(
	ctx context.Context,
	userEmail string,
	path string,
) (db.AgentSession, error) {
	resolvedPath, err := s.validatePiSessionPath(path)
	if err != nil {
		return db.AgentSession{}, err
	}
	scan, err := ScanPiSessionJSONL(resolvedPath, PiSessionScanOptions{
		BatchSize:    defaultPiSessionImportBatchSize,
		ThoughtsRoot: s.thoughtsRoot,
	})
	if err != nil {
		return db.AgentSession{}, err
	}

	inference := scan.Inference
	inference.PlanDir = s.remapSessionIndexPlanDir(inference.PlanDir, scan.Header.Cwd)
	if inference.PlanDir == "" {
		remappedCandidates := collections.NewSet[string]()
		for _, candidate := range inference.Candidates {
			if planDir, ok := s.remapCopiedWorkspacePlanDir(candidate); ok {
				remappedCandidates.Add(planDir)
			}
		}
		candidates := sortedSetValues(remappedCandidates)
		inference.Candidates = candidates
		switch len(candidates) {
		case 1:
			inference.PlanDir = candidates[0]
			inference.Status = "single-plan"
		case 0:
			if strings.TrimSpace(inference.Status) == "" {
				inference.Status = "none"
			}
		default:
			inference.Status = "ambiguous"
		}
	}

	scan.Inference = inference
	status := statusForInference(inference)
	lastError := sql.NullString{}
	if status == "ambiguous" {
		lastError = nullString("multiple candidate plan workspaces")
	}

	return s.queries.UpsertAgentSessionIndex(ctx, db.UpsertAgentSessionIndexParams{
		ID:                  uuid.NewString(),
		WorkspaceID:         nullableString(inference.WorkspaceID),
		ThreadID:            sql.NullString{},
		UserEmail:           nullableString(userEmail),
		Source:              string(AgentSessionSourceTerminal),
		SessionPath:         nullableString(resolvedPath),
		SessionID:           nullableString(scan.Header.ID),
		ParentSessionID:     nullableString(scan.Header.ParentSession),
		Cwd:                 nullableString(scan.Header.Cwd),
		Status:              status,
		InferredWorkspaceID: nullableString(inference.WorkspaceID),
		InferredPlanDir:     nullableString(inference.PlanDir),
		LastError:           lastError,
		MetadataJson: nullableString(
			importMetadataJSONWithSummary(
				scan,
				PiSessionImportStats{
					LinesRead:   scan.LinesRead,
					EntriesRead: scan.EntriesRead,
					BatchCount:  scan.BatchCount,
				},
			),
		),
	})
}

func (s *Service) remapSessionIndexPlanDir(planDir, cwd string) string {
	candidates := []string{planDir, cwd}
	for _, candidate := range candidates {
		if remapped, ok := s.remapCopiedWorkspacePlanDir(candidate); ok {
			return remapped
		}
	}
	return ""
}

func (s *Service) remapCopiedWorkspacePlanDir(candidate string) (string, bool) {
	clean := strings.TrimSpace(candidate)
	if clean == "" {
		return "", false
	}
	if planDir, ok := s.canonicalPlanDirFromSource(clean); ok {
		return planDir, true
	}
	if s.thoughtsRoot == "" {
		return "", false
	}
	marker := string(filepath.Separator) + "thoughts" + string(filepath.Separator)
	idx := strings.Index(clean, marker)
	if idx < 0 {
		return "", false
	}
	rel := clean[idx+len(marker):]
	rebuilt := filepath.Join(s.thoughtsRoot, rel)
	return s.canonicalPlanDirFromSource(rebuilt)
}

func (s *Service) planSidebarNotifyKey(userEmail string) string {
	return "plan-sidebar:" + normalizePlanSidebarUser(userEmail)
}

func (s *Service) NotifyPlanSidebar(userEmail string) WorkspaceStreamSignal {
	if s.notifier == nil {
		return WorkspaceStreamSignal{}
	}
	return s.notifier.Notify(
		s.planSidebarNotifyKey(userEmail),
		WorkspaceStreamSignal{Scope: PatchWorkspaceSidebar},
	)
}

func (s *Service) projectPlanSidebarNotifyKey() string {
	return projectPlanSidebarNotifyKey(s.projectName)
}

func (s *Service) NotifyProjectPlanSidebar() WorkspaceStreamSignal {
	if s.notifier == nil {
		return WorkspaceStreamSignal{}
	}
	return s.notifier.NotifyProjectPlanSidebar(s.projectName)
}

func normalizePlanSidebarUser(userEmail string) string {
	return strings.TrimSpace(strings.ToLower(userEmail))
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func sortedSetValues(set collections.Set[string]) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	if len(values) > 1 {
		for i := 1; i < len(values); i++ {
			for j := i; j > 0 && values[j] < values[j-1]; j-- {
				values[j], values[j-1] = values[j-1], values[j]
			}
		}
	}
	return values
}

func (r PiSessionIndexResult) String() string {
	return fmt.Sprintf(
		"indexed=%d imported=%d skipped=%d failed=%d",
		r.Indexed,
		r.Imported,
		r.Skipped,
		r.Failed,
	)
}
