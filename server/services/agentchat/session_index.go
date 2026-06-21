package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
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

	err = walkPiSessionFilesBounded(ctx, root, 0, 0, 0, func(path string, _ fs.FileInfo) error {
		record, indexErr := s.indexPiSessionFile(ctx, user, path)
		if indexErr != nil {
			result.Failed++
			return nil //nolint:nilerr // Keep indexing best-effort across corrupt session files.
		}
		result.Indexed++
		if record.PlanDir.Valid && record.PlanDir.String != "" {
			affectedPlans.Add(record.PlanDir.String)
		}
		if record.AttachedWorkspaceID.Valid && record.AttachedWorkspaceID.String != "" {
			affectedWorkspaces.Add(record.AttachedWorkspaceID.String)
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
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return db.AgentSession{}, err
	}
	hash, err := fileSHA256(resolvedPath)
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

	scan.Inference = s.remapPiSessionScanInference(scan)
	inference := scan.Inference
	status := statusForInference(inference)
	lastError := sql.NullString{}
	if status == "ambiguous" {
		lastError = nullString("multiple candidate plan workspaces")
	}

	return s.queries.UpsertAgentSessionIndex(ctx, db.UpsertAgentSessionIndexParams{
		ID:                     uuid.NewString(),
		IdentityKind:           "global_pi",
		ArtifactPath:           nullableString(resolvedPath),
		PlanDir:                nullableString(inference.PlanDir),
		ParentPlanDir:          sql.NullString{},
		SourceReviewDir:        sql.NullString{},
		Agent:                  defaultAgentSessionAgent,
		ExternalSessionID:      nullableString(scan.Header.ID),
		ParentSessionID:        nullableString(scan.Header.ParentSession),
		Cwd:                    nullableString(scan.Header.Cwd),
		WorkflowID:             sql.NullString{},
		WorkflowNodeID:         sql.NullString{},
		ContinuedFromSessionID: sql.NullString{},
		ForkedFromSessionID:    sql.NullString{},
		FileSize:               info.Size(),
		FileMtime:              sql.NullTime{Time: info.ModTime(), Valid: true},
		FileHash:               nullableString(hash),
		LastIndexedOffset:      info.Size(),
		ProjectionState:        status,
		ProjectedThreadID:      sql.NullString{},
		IndexedByUserEmail:     nullableString(userEmail),
		AttachedWorkspaceID:    nullableString(inference.WorkspaceID),
		LastError:              lastError,
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

type TerminalSessionAdoptionResult struct {
	ImportedSessions       int
	AdoptedQRSPIWorkspaces int
	Changed                bool
}

type LegacyPiSessionScanInput struct {
	UserEmail   string
	MaxFiles    int
	MaxBytes    int64
	MaxDuration time.Duration
	Force       bool
}

func (s *Service) ImportAdoptablePiSessions(
	ctx context.Context,
) (TerminalSessionAdoptionResult, error) {
	return s.ImportAdoptablePiSessionsWithInput(ctx, LegacyPiSessionScanInput{})
}

// ImportAdoptablePiSessionsWithInput is a manual/backfill fallback only. Scheduled sync
// must use IndexTerminalMetadata and ApplyQRSPIProjections instead of transcript import.
func (s *Service) ImportAdoptablePiSessionsWithInput(
	ctx context.Context,
	input LegacyPiSessionScanInput,
) (TerminalSessionAdoptionResult, error) {
	root := strings.TrimSpace(s.piSessionsDir)
	if root == "" {
		return TerminalSessionAdoptionResult{}, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TerminalSessionAdoptionResult{}, nil
		}
		return TerminalSessionAdoptionResult{}, err
	}
	if !info.IsDir() {
		return TerminalSessionAdoptionResult{}, nil
	}

	result := TerminalSessionAdoptionResult{}
	affectedWorkspaces := collections.NewSet[string]()
	err = walkPiSessionFilesBounded(
		ctx,
		root,
		input.MaxFiles,
		input.MaxBytes,
		input.MaxDuration,
		func(path string, _ fs.FileInfo) error {
			adopted, importErr := s.importAdoptablePiSession(ctx, path)
			if importErr != nil {
				log.Printf("terminal_session_auto_import_failed path=%q: %v", path, importErr)
				return nil
			}
			if adopted.WorkspaceID != "" {
				affectedWorkspaces.Add(adopted.WorkspaceID)
			}
			result.ImportedSessions += adopted.ImportedSessions
			result.AdoptedQRSPIWorkspaces += adopted.AdoptedQRSPIWorkspaces
			result.Changed = result.Changed || adopted.Changed
			return nil
		},
	)
	if err != nil {
		return result, err
	}
	if s.notifier != nil {
		for _, workspaceID := range sortedSetValues(affectedWorkspaces) {
			s.notifier.NotifyWorkspaceResource(workspaceID)
		}
	}
	return result, nil
}

type piSessionAdoptionFileResult struct {
	WorkspaceID            string
	ImportedSessions       int
	AdoptedQRSPIWorkspaces int
	Changed                bool
}

func (s *Service) importAdoptablePiSession(
	ctx context.Context,
	path string,
) (piSessionAdoptionFileResult, error) {
	resolvedPath, err := s.validatePiSessionPath(path)
	if err != nil {
		return piSessionAdoptionFileResult{}, err
	}
	scan, err := ScanPiSessionJSONL(resolvedPath, PiSessionScanOptions{
		BatchSize:    defaultPiSessionImportBatchSize,
		ThoughtsRoot: s.thoughtsRoot,
	})
	if err != nil {
		return piSessionAdoptionFileResult{}, err
	}
	scan.Inference = s.remapPiSessionScanInference(scan)
	status := statusForInference(scan.Inference)
	if status == "unassigned" || status == "ambiguous" {
		return piSessionAdoptionFileResult{}, nil
	}
	workspace, ok, err := s.resolveImportWorkspaceFromScan(
		ctx,
		s.queries,
		SessionImportInput{SessionPath: resolvedPath, Source: AgentSessionSourceTerminal},
		scan,
	)
	if err != nil {
		return piSessionAdoptionFileResult{}, fmt.Errorf("resolve import workspace: %w", err)
	}
	if !ok {
		return piSessionAdoptionFileResult{}, nil
	}
	if WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType)) != WorkspaceWorkflowQRSPI {
		return piSessionAdoptionFileResult{}, nil
	}
	imported, err := s.ImportPiSession(ctx, SessionImportInput{
		SessionPath:          resolvedPath,
		Source:               AgentSessionSourceTerminal,
		ExplicitWorkspaceID:  workspace.ID,
		ExplicitWorkspaceDir: workspace.RootDocPath,
		UserEmail:            workspace.UserEmail,
	})
	if err != nil {
		return piSessionAdoptionFileResult{}, fmt.Errorf("import pi session: %w", err)
	}
	fileResult := piSessionAdoptionFileResult{WorkspaceID: workspace.ID}
	if imported.Status == "hydrated" || imported.Status == "imported" || imported.Status == "diverged" {
		fileResult.ImportedSessions = 1
		if imported.Stats.EntriesImported > 0 || imported.Diverged {
			fileResult.Changed = true
		}
	}
	if imported.ThreadID == "" || imported.ImportedHeadEntry == "" {
		return fileResult, nil
	}
	adopted, err := s.AdoptImportedQRSPIState(
		ctx,
		workspace.ID,
		imported.ThreadID,
		imported.ImportedHeadEntry,
	)
	if err != nil {
		return fileResult, fmt.Errorf("adopt imported qrspi state: %w", err)
	}
	if adopted {
		fileResult.AdoptedQRSPIWorkspaces = 1
		fileResult.Changed = true
	}
	return fileResult, nil
}

func walkPiSessionFilesBounded(
	ctx context.Context,
	root string,
	maxFiles int,
	maxBytes int64,
	maxDuration time.Duration,
	visit func(path string, info fs.FileInfo) error,
) error {
	if visit == nil {
		return nil
	}
	started := time.Now()
	var files int
	var bytes int64
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if maxDuration > 0 && time.Since(started) >= maxDuration {
			return fs.SkipAll
		}
		if d.IsDir() || filepath.Ext(path) != jsonlExtension {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if maxFiles > 0 && files >= maxFiles {
			return fs.SkipAll
		}
		if maxBytes > 0 && bytes+info.Size() > maxBytes {
			return fs.SkipAll
		}
		files++
		bytes += info.Size()
		return visit(path, info)
	})
}

func (s *Service) remapPiSessionScanInference(
	scan PiSessionScanSummary,
) WorkspaceInferenceResult {
	inference := scan.Inference
	inference.PlanDir = s.remapSessionIndexPlanDir(inference.PlanDir, scan.Header.Cwd)
	if inference.PlanDir != "" {
		if strings.TrimSpace(inference.Status) == "" || inference.Status == "none" {
			inference.Status = "single-plan"
		}
		return inference
	}
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
	return inference
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
