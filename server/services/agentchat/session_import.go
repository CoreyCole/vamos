package agentchat

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/agents/workflows/qrspi"
	wruntime "github.com/CoreyCole/vamos/pkg/agents/workflows/runtime"
	"github.com/CoreyCole/vamos/pkg/db"
	agentchatworkflows "github.com/CoreyCole/vamos/server/services/agentchat/workflows"
)

const (
	defaultPiSessionImportBatchSize = 200
	initialPiSessionScanBufferBytes = 64 * 1024
	maxPiSessionScanTokenBytes      = 16 * 1024 * 1024
)

type PiSessionDocument struct {
	Header  PiSessionHeader  `json:"header"`
	Entries []PiSessionEntry `json:"entries"`
}

type PiSessionHeader struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	Cwd           string `json:"cwd"`
	ParentSession string `json:"parentSession"`
}

type PiSessionEntry struct {
	Type      string           `json:"type"`
	ID        string           `json:"id"`
	ParentID  string           `json:"parentId"`
	Timestamp string           `json:"timestamp"`
	Message   PiSessionMessage `json:"message"`
	Raw       json.RawMessage  `json:"-"`
}

type PiSessionMessage struct {
	Role       string `json:"role"`
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Content    any    `json:"content"`
	Details    any    `json:"details"`
	IsError    bool   `json:"isError"`
}

type PiSessionScanOptions struct {
	BatchSize            int
	ThoughtsRoot         string
	ExplicitWorkspaceID  string
	ExplicitWorkspaceDir string
}

//nolint:tagliatelle // Stats are persisted in existing snake_case metadata JSON.
type PiSessionScanSummary struct {
	Header        PiSessionHeader          `json:"header"`
	Inference     WorkspaceInferenceResult `json:"inference"`
	LinesRead     int                      `json:"lines_read"`
	EntriesRead   int                      `json:"entries_read"`
	FinalEntryID  string                   `json:"final_entry_id"`
	BatchCount    int                      `json:"batch_count"`
	FirstUserText string                   `json:"first_user_text,omitempty"`
}

type PiSessionEntryBatch struct {
	Entries          []PiSessionEntry
	StartOriginOrder int64
	LineStart        int
	LineEnd          int
}

//nolint:tagliatelle // Stats are persisted in existing snake_case metadata JSON.
type PiSessionImportStats struct {
	LinesRead       int   `json:"lines_read"`
	EntriesRead     int   `json:"entries_read"`
	EntriesImported int   `json:"entries_imported"`
	EntriesSkipped  int   `json:"entries_skipped"`
	BatchCount      int   `json:"batch_count"`
	FailedLine      int   `json:"failed_line,omitempty"`
	DurationMS      int64 `json:"duration_ms,omitempty"`
}

//nolint:tagliatelle // Persisted JSON uses snake_case fields.
type sessionImportMetadata struct {
	Inference     WorkspaceInferenceResult `json:"inference"`
	Stats         PiSessionImportStats     `json:"stats"`
	Header        PiSessionHeader          `json:"header,omitempty"`
	FirstUserText string                   `json:"first_user_text,omitempty"`
}

type WorkspaceInferenceResult struct {
	WorkspaceID string   `json:"workspace_id,omitempty"`
	PlanDir     string   `json:"plan_dir,omitempty"`
	Status      string   `json:"status"`
	Candidates  []string `json:"candidates,omitempty"`
}

func ParsePiSessionJSONL(path string) (PiSessionDocument, error) {
	file, err := os.Open(path)
	if err != nil {
		return PiSessionDocument{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(
		make([]byte, 0, initialPiSessionScanBufferBytes),
		maxPiSessionScanTokenBytes,
	)
	lineNumber := 0
	doc := PiSessionDocument{Entries: []PiSessionEntry{}}
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if lineNumber == 1 {
			if err := json.Unmarshal([]byte(line), &doc.Header); err != nil {
				return PiSessionDocument{}, fmt.Errorf("parse session header: %w", err)
			}
			if doc.Header.Type != "session" || strings.TrimSpace(doc.Header.ID) == "" {
				return PiSessionDocument{}, fmt.Errorf(
					"first JSONL line must be a session header with id",
				)
			}
			continue
		}
		entry, err := parsePiSessionEntryLine(line, lineNumber)
		if err != nil {
			return PiSessionDocument{}, err
		}
		doc.Entries = append(doc.Entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return PiSessionDocument{}, fmt.Errorf("read session JSONL: %w", err)
	}
	if lineNumber == 0 {
		return PiSessionDocument{}, fmt.Errorf("session JSONL is empty")
	}
	if err := ValidatePiSessionDocument(doc); err != nil {
		return PiSessionDocument{}, err
	}
	return doc, nil
}

func ScanPiSessionJSONL(
	path string,
	opts PiSessionScanOptions,
) (PiSessionScanSummary, error) {
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultPiSessionImportBatchSize
	}
	file, err := os.Open(path)
	if err != nil {
		return PiSessionScanSummary{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(
		make([]byte, 0, initialPiSessionScanBufferBytes),
		maxPiSessionScanTokenBytes,
	)
	seen := map[string]struct{}{}
	toolCalls := map[string]toolCallRef{}
	candidates := map[string]struct{}{}
	summary := PiSessionScanSummary{}

	for scanner.Scan() {
		summary.LinesRead++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if summary.LinesRead == 1 {
			if err := json.Unmarshal([]byte(line), &summary.Header); err != nil {
				return PiSessionScanSummary{}, fmt.Errorf("parse session header: %w", err)
			}
			if summary.Header.Type != "session" ||
				strings.TrimSpace(summary.Header.ID) == "" {
				return PiSessionScanSummary{}, errors.New(
					"first JSONL line must be a session header with id",
				)
			}
			continue
		}
		entry, err := parsePiSessionEntryLine(line, summary.LinesRead)
		if err != nil {
			return PiSessionScanSummary{}, err
		}
		if err := validateStreamingPiEntry(entry, summary.EntriesRead, seen); err != nil {
			return PiSessionScanSummary{}, err
		}
		seen[entry.ID] = struct{}{}
		summary.EntriesRead++
		summary.FinalEntryID = entry.ID
		if summary.FirstUserText == "" &&
			strings.TrimSpace(entry.Message.Role) == "user" {
			summary.FirstUserText = strings.TrimSpace(
				extractContentText(entry.Message.Content),
			)
		}
		collectTouchedPlanDirsFromEntry(
			entry,
			opts.ThoughtsRoot,
			summary.Header.Cwd,
			toolCalls,
			candidates,
		)
	}
	if err := scanner.Err(); err != nil {
		return PiSessionScanSummary{}, fmt.Errorf("read session JSONL: %w", err)
	}
	if summary.LinesRead == 0 {
		return PiSessionScanSummary{}, errors.New("session JSONL is empty")
	}
	summary.BatchCount = (summary.EntriesRead + opts.BatchSize - 1) / opts.BatchSize
	inference, err := inferenceFromScan(summary.Header, candidates, opts)
	if err != nil {
		return PiSessionScanSummary{}, err
	}
	summary.Inference = inference
	return summary, nil
}

func parsePiSessionEntryLine(line string, lineNumber int) (PiSessionEntry, error) {
	var entry PiSessionEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return PiSessionEntry{}, fmt.Errorf(
			"parse session entry line %d: %w",
			lineNumber,
			err,
		)
	}
	entry.Raw = append(entry.Raw[:0], line...)
	return entry, nil
}

func validateStreamingPiEntry(
	entry PiSessionEntry,
	idx int,
	seen map[string]struct{},
) error {
	if strings.TrimSpace(entry.ID) == "" {
		return fmt.Errorf("entry %d is missing id", idx+1)
	}
	if _, ok := seen[entry.ID]; ok {
		return fmt.Errorf("duplicate entry id %q", entry.ID)
	}
	if idx > 0 && strings.TrimSpace(entry.ParentID) == "" {
		return fmt.Errorf("entry %q is missing parentId", entry.ID)
	}
	if strings.TrimSpace(entry.ParentID) != "" {
		if _, ok := seen[entry.ParentID]; !ok {
			return fmt.Errorf(
				"entry %q references missing parent %q",
				entry.ID,
				entry.ParentID,
			)
		}
	}
	if strings.TrimSpace(entry.Type) == "" {
		return fmt.Errorf("entry %q is missing type", entry.ID)
	}
	if strings.TrimSpace(entry.Timestamp) == "" {
		return fmt.Errorf("entry %q is missing timestamp", entry.ID)
	}
	return nil
}

func StreamPiSessionJSONLBatches(
	path string,
	batchSize int,
	fn func(PiSessionEntryBatch) error,
) (PiSessionImportStats, error) {
	if batchSize <= 0 {
		batchSize = defaultPiSessionImportBatchSize
	}
	started := time.Now()
	file, err := os.Open(path)
	if err != nil {
		return PiSessionImportStats{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(
		make([]byte, 0, initialPiSessionScanBufferBytes),
		maxPiSessionScanTokenBytes,
	)
	stats := PiSessionImportStats{}
	batch := PiSessionEntryBatch{Entries: make([]PiSessionEntry, 0, batchSize)}
	flush := func() error {
		if len(batch.Entries) == 0 {
			return nil
		}
		stats.BatchCount++
		if err := fn(batch); err != nil {
			stats.FailedLine = batch.LineStart
			return err
		}
		batch = PiSessionEntryBatch{Entries: make([]PiSessionEntry, 0, batchSize)}
		return nil
	}

	for scanner.Scan() {
		stats.LinesRead++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if stats.LinesRead == 1 {
			continue
		}
		entry, err := parsePiSessionEntryLine(line, stats.LinesRead)
		if err != nil {
			stats.FailedLine = stats.LinesRead
			stats.DurationMS = time.Since(started).Milliseconds()
			return stats, err
		}
		if len(batch.Entries) == 0 {
			batch.StartOriginOrder = int64(stats.EntriesRead)
			batch.LineStart = stats.LinesRead
		}
		batch.LineEnd = stats.LinesRead
		batch.Entries = append(batch.Entries, entry)
		stats.EntriesRead++
		if len(batch.Entries) != batchSize {
			continue
		}
		if err := flush(); err != nil {
			stats.DurationMS = time.Since(started).Milliseconds()
			return stats, err
		}
	}
	if err := scanner.Err(); err != nil {
		stats.DurationMS = time.Since(started).Milliseconds()
		return stats, fmt.Errorf("read session JSONL: %w", err)
	}
	if err := flush(); err != nil {
		stats.DurationMS = time.Since(started).Milliseconds()
		return stats, err
	}
	stats.DurationMS = time.Since(started).Milliseconds()
	return stats, nil
}

func StreamPiSessionJSONL(
	path string,
	batchSize int,
	fn func(batch []PiSessionEntry) error,
) (PiSessionImportStats, error) {
	return StreamPiSessionJSONLBatches(
		path,
		batchSize,
		func(batch PiSessionEntryBatch) error { return fn(batch.Entries) },
	)
}

func ValidatePiSessionDocument(doc PiSessionDocument) error {
	if doc.Header.Type != "session" || strings.TrimSpace(doc.Header.ID) == "" {
		return fmt.Errorf("invalid session header")
	}
	seen := map[string]struct{}{}
	for idx, entry := range doc.Entries {
		if err := validateStreamingPiEntry(entry, idx, seen); err != nil {
			return err
		}
		seen[entry.ID] = struct{}{}
	}
	return nil
}

func DetectTouchedPlanDirs(
	entries []PiSessionEntry,
	thoughtsRoot string,
) ([]string, error) {
	return detectTouchedPlanDirs(entries, thoughtsRoot, "")
}

func detectTouchedPlanDirs(
	entries []PiSessionEntry,
	thoughtsRoot, cwd string,
) ([]string, error) {
	toolCalls := map[string]toolCallRef{}
	candidates := map[string]struct{}{}
	for _, entry := range entries {
		collectTouchedPlanDirsFromEntry(entry, thoughtsRoot, cwd, toolCalls, candidates)
	}
	plans := make([]string, 0, len(candidates))
	for plan := range candidates {
		plans = append(plans, plan)
	}
	sort.Strings(plans)
	return plans, nil
}

func (s *Service) InferWorkspaceFromSession(
	doc PiSessionDocument,
	explicitWorkspaceID, explicitWorkspaceDir string,
) (WorkspaceInferenceResult, error) {
	plans, err := detectTouchedPlanDirs(doc.Entries, s.thoughtsRoot, doc.Header.Cwd)
	if err != nil {
		return WorkspaceInferenceResult{}, err
	}
	candidates := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		candidates[plan] = struct{}{}
	}
	return inferenceFromScan(doc.Header, candidates, PiSessionScanOptions{
		ThoughtsRoot:         s.thoughtsRoot,
		ExplicitWorkspaceID:  explicitWorkspaceID,
		ExplicitWorkspaceDir: explicitWorkspaceDir,
	})
}

func collectTouchedPlanDirsFromEntry(
	entry PiSessionEntry,
	thoughtsRoot string,
	cwd string,
	toolCalls map[string]toolCallRef,
	candidates map[string]struct{},
) {
	if strings.TrimSpace(entry.Message.Role) == "assistant" {
		for _, call := range extractToolCalls(entry.Message.Content) {
			if call.ID != "" {
				toolCalls[call.ID] = call
			}
		}
		return
	}
	if strings.TrimSpace(entry.Message.Role) != "toolResult" || entry.Message.IsError {
		return
	}
	toolName := strings.TrimSpace(entry.Message.ToolName)
	if toolName != "write" && toolName != "edit" {
		return
	}
	values := []any{entry.Message.Details, entry.Message.Content}
	if call, ok := toolCalls[strings.TrimSpace(entry.Message.ToolCallID)]; ok {
		if call.Name != "" && call.Name != toolName {
			return
		}
		values = append(values, call.Arguments)
	}
	for _, rawPath := range extractPathStrings(values...) {
		planDir, ok := planDirForTouchedPath(rawPath, thoughtsRoot, cwd)
		if ok {
			candidates[planDir] = struct{}{}
		}
	}
}

func inferenceFromScan(
	header PiSessionHeader,
	candidates map[string]struct{},
	opts PiSessionScanOptions,
) (WorkspaceInferenceResult, error) {
	_ = header
	if strings.TrimSpace(opts.ExplicitWorkspaceID) != "" {
		return WorkspaceInferenceResult{
			WorkspaceID: strings.TrimSpace(opts.ExplicitWorkspaceID),
			PlanDir:     strings.TrimSpace(opts.ExplicitWorkspaceDir),
			Status:      "explicit",
		}, nil
	}
	if strings.TrimSpace(opts.ExplicitWorkspaceDir) != "" {
		root, err := ValidateWorkspaceRootDocPath(
			opts.ExplicitWorkspaceDir,
			opts.ThoughtsRoot,
			"",
		)
		if err != nil {
			return WorkspaceInferenceResult{}, err
		}
		return WorkspaceInferenceResult{PlanDir: root, Status: "explicit"}, nil
	}
	plans := make([]string, 0, len(candidates))
	for plan := range candidates {
		plans = append(plans, plan)
	}
	sort.Strings(plans)
	switch len(plans) {
	case 0:
		return WorkspaceInferenceResult{Status: "none"}, nil
	case 1:
		return WorkspaceInferenceResult{
			PlanDir:    plans[0],
			Status:     "single-plan",
			Candidates: plans,
		}, nil
	default:
		return WorkspaceInferenceResult{Status: "ambiguous", Candidates: plans}, nil
	}
}

func (s *Service) ImportPiSession(
	ctx context.Context,
	input SessionImportInput,
) (SessionImportResult, error) {
	resolvedPath, err := s.validatePiSessionPath(input.SessionPath)
	if err != nil {
		return SessionImportResult{}, err
	}
	source := input.Source
	if source == "" {
		source = AgentSessionSourceTerminal
	}

	scan, scanErr := ScanPiSessionJSONL(resolvedPath, PiSessionScanOptions{
		BatchSize:            defaultPiSessionImportBatchSize,
		ThoughtsRoot:         s.thoughtsRoot,
		ExplicitWorkspaceID:  input.ExplicitWorkspaceID,
		ExplicitWorkspaceDir: input.ExplicitWorkspaceDir,
	})
	if scanErr != nil {
		if err := s.validateExplicitImportWorkspaceContext(
			ctx,
			s.queries,
			input,
		); err != nil {
			return SessionImportResult{}, err
		}
		session, existed, err := s.ensureImportSession(
			ctx,
			s.queries,
			resolvedPath,
			source,
			PiSessionHeader{},
			"failed",
			"",
			"",
			scanErr.Error(),
			"",
			input.UserEmail,
		)
		if err != nil {
			return SessionImportResult{}, err
		}
		if existed {
			if err := s.validateImportSessionReuse(
				ctx,
				s.queries,
				session,
				db.Workspace{},
				false,
				input.UserEmail,
			); err != nil {
				return SessionImportResult{}, err
			}
		}
		stats := PiSessionImportStats{FailedLine: scan.LinesRead}
		_ = s.queries.UpdateAgentSessionImportFailedState(
			ctx,
			db.UpdateAgentSessionImportFailedStateParams{
				ID:        session.ID,
				LastError: nullString(scanErr.Error()),
				MetadataJson: nullString(
					importMetadataJSONWithStats(WorkspaceInferenceResult{}, stats),
				),
			},
		)
		return SessionImportResult{
			SessionID: session.ID,
			Status:    "failed",
			Stats:     stats,
		}, scanErr
	}
	stats := PiSessionImportStats{
		LinesRead:   scan.LinesRead,
		EntriesRead: scan.EntriesRead,
		BatchCount:  scan.BatchCount,
	}
	metadata := importMetadataJSONWithSummary(scan, stats)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionImportResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)

	workspace, workspaceOK, err := s.resolveImportWorkspaceFromScan(ctx, q, input, scan)
	if err != nil {
		return SessionImportResult{}, err
	}
	status := statusForInference(scan.Inference)
	session, existed, err := s.ensureImportSession(
		ctx,
		q,
		resolvedPath,
		source,
		scan.Header,
		status,
		maybeWorkspaceID(workspace, workspaceOK),
		scan.Inference.PlanDir,
		"",
		metadata,
		input.UserEmail,
	)
	if err != nil {
		return SessionImportResult{}, err
	}
	if existed {
		if err := s.validateImportSessionReuse(
			ctx,
			q,
			session,
			workspace,
			workspaceOK,
			input.UserEmail,
		); err != nil {
			return SessionImportResult{}, err
		}
	}
	if !workspaceOK {
		if err := q.UpdateAgentSessionInferenceState(
			ctx,
			db.UpdateAgentSessionInferenceStateParams{
				ID:                  session.ID,
				WorkspaceID:         sql.NullString{},
				ThreadID:            sql.NullString{},
				Status:              status,
				InferredWorkspaceID: sql.NullString{},
				InferredPlanDir:     nullString(scan.Inference.PlanDir),
				LastError:           sql.NullString{},
				MetadataJson:        nullString(metadata),
			},
		); err != nil {
			return SessionImportResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return SessionImportResult{}, err
		}
		return SessionImportResult{
			SessionID: session.ID,
			Status:    status,
			Stats:     stats,
		}, nil
	}

	thread, diverged, err := s.resolveImportThreadFromScan(
		ctx,
		q,
		session,
		workspace,
		scan,
	)
	if err != nil {
		return SessionImportResult{}, err
	}
	if err := q.UpdateAgentSessionImportingState(
		ctx,
		db.UpdateAgentSessionImportingStateParams{
			ID:                  session.ID,
			WorkspaceID:         nullString(workspace.ID),
			ThreadID:            nullString(thread.ID),
			InferredWorkspaceID: nullString(workspace.ID),
			InferredPlanDir:     nullString(scan.Inference.PlanDir),
			MetadataJson:        nullString(metadata),
		},
	); err != nil {
		return SessionImportResult{}, err
	}
	if err := q.AttachThreadToWorkspace(ctx, db.AttachThreadToWorkspaceParams{ID: thread.ID, WorkspaceID: nullString(workspace.ID)}); err != nil {
		return SessionImportResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionImportResult{}, err
	}

	var imported, skipped int
	streamStats, batchErr := StreamPiSessionJSONLBatches(
		resolvedPath,
		defaultPiSessionImportBatchSize,
		func(batch PiSessionEntryBatch) error {
			batchTx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer func() { _ = batchTx.Rollback() }()
			batchQ := s.queries.WithTx(batchTx)
			batchImported, batchSkipped, err := s.importSessionEntryBatch(
				ctx,
				batchQ,
				session,
				thread,
				batch,
			)
			if err != nil {
				return err
			}
			if err := batchTx.Commit(); err != nil {
				return err
			}
			imported += batchImported
			skipped += batchSkipped
			return nil
		},
	)
	streamStats.EntriesImported = imported
	streamStats.EntriesSkipped = skipped
	stats = streamStats
	metadata = importMetadataJSONWithSummary(scan, stats)
	if batchErr != nil {
		_ = s.queries.UpdateAgentSessionImportFailedState(
			ctx,
			db.UpdateAgentSessionImportFailedStateParams{
				ID:           session.ID,
				LastError:    nullString(batchErr.Error()),
				MetadataJson: nullString(metadata),
			},
		)
		return SessionImportResult{
			SessionID:   session.ID,
			WorkspaceID: workspace.ID,
			ThreadID:    thread.ID,
			Status:      "failed",
			Stats:       stats,
		}, batchErr
	}

	status = "imported"
	if diverged {
		status = "diverged"
	}
	tx, err = s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionImportResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	q = s.queries.WithTx(tx)
	if err := q.UpdateAgentSessionImportFinalState(
		ctx,
		db.UpdateAgentSessionImportFinalStateParams{
			ID:                  session.ID,
			WorkspaceID:         nullString(workspace.ID),
			ThreadID:            nullString(thread.ID),
			Status:              status,
			InferredWorkspaceID: nullString(workspace.ID),
			InferredPlanDir:     nullString(scan.Inference.PlanDir),
			ImportedHeadEntryID: nullString(scan.FinalEntryID),
			MetadataJson:        nullString(metadata),
		},
	); err != nil {
		return SessionImportResult{}, err
	}
	if err := q.UpdateAgentThreadHead(
		ctx,
		db.UpdateAgentThreadHeadParams{
			ID:          thread.ID,
			HeadEntryID: nullString(scan.FinalEntryID),
		},
	); err != nil {
		return SessionImportResult{}, err
	}
	if err := q.AttachThreadToWorkspace(ctx, db.AttachThreadToWorkspaceParams{ID: thread.ID, WorkspaceID: nullString(workspace.ID)}); err != nil {
		return SessionImportResult{}, err
	}
	_ = q.UpdateWorkspaceSelectedThread(
		ctx,
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspace.ID,
			SelectedThreadID: nullString(thread.ID),
		},
	)
	eventType := "session_imported"
	if diverged {
		eventType = "session_import_diverged"
	}
	event, err := s.AppendWorkspaceEvent(ctx, q, AppendWorkspaceEventInput{
		WorkspaceID: workspace.ID,
		EventType:   eventType,
		ActorType:   "system",
		ThreadID:    thread.ID,
		SessionID:   session.ID,
		PayloadJSON: metadata,
		EventKey:    fmt.Sprintf("%s:%s:%s", eventType, session.ID, scan.FinalEntryID),
	})
	if err != nil {
		return SessionImportResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionImportResult{}, err
	}
	if changes, err := s.SyncWorkspaceDocInventory(
		ctx,
		workspace,
	); err == nil && len(changes) > 0 {
		s.NotifyWorkspaceForEvent(db.WorkspaceEvent{
			WorkspaceID: workspace.ID,
			EventType:   "artifact_updated",
		})
	}
	s.NotifyWorkspaceForEvent(event)
	return SessionImportResult{
		SessionID:         session.ID,
		WorkspaceID:       workspace.ID,
		ThreadID:          thread.ID,
		ImportedHeadEntry: scan.FinalEntryID,
		Status:            status,
		Diverged:          diverged,
		Stats:             stats,
	}, nil
}

func (s *Service) importSessionEntries(
	ctx context.Context,
	q *db.Queries,
	session db.AgentSession,
	thread db.AgentThread,
	doc PiSessionDocument,
) (string, error) {
	if len(doc.Entries) == 0 {
		return "", nil
	}
	for idx, entry := range doc.Entries {
		timestamp, err := parsePiTimestamp(entry.Timestamp)
		if err != nil {
			return "", fmt.Errorf("entry %q timestamp: %w", entry.ID, err)
		}
		payload := string(entry.Raw)
		if payload == "" {
			payloadBytes, err := json.Marshal(entry)
			if err != nil {
				return "", err
			}
			payload = string(payloadBytes)
		}
		err = q.CreateAgentEntry(ctx, db.CreateAgentEntryParams{
			LineageID:        thread.LineageID,
			EntryID:          entry.ID,
			ParentEntryID:    nullString(entry.ParentID),
			EntryType:        normalizePiEntryType(entry.Type),
			OriginOrder:      int64(idx),
			PayloadJson:      payload,
			OriginThreadID:   thread.ID,
			OriginRunID:      sql.NullString{},
			OriginSessionID:  nullString(session.ID),
			SessionTimestamp: timestamp,
		})
		if err != nil && !isUniqueConstraintError(err) {
			return "", err
		}
	}
	return doc.Entries[len(doc.Entries)-1].ID, nil
}

func (s *Service) importSessionEntryBatch(
	ctx context.Context,
	q *db.Queries,
	session db.AgentSession,
	thread db.AgentThread,
	batch PiSessionEntryBatch,
) (int, int, error) {
	imported, skipped := 0, 0
	for idx, entry := range batch.Entries {
		timestamp, err := parsePiTimestamp(entry.Timestamp)
		if err != nil {
			return imported, skipped, fmt.Errorf("entry %q timestamp: %w", entry.ID, err)
		}
		payload := string(entry.Raw)
		if payload == "" {
			payloadBytes, err := json.Marshal(entry)
			if err != nil {
				return imported, skipped, err
			}
			payload = string(payloadBytes)
		}
		err = q.CreateAgentEntry(ctx, db.CreateAgentEntryParams{
			LineageID:        thread.LineageID,
			EntryID:          entry.ID,
			ParentEntryID:    nullString(entry.ParentID),
			EntryType:        normalizePiEntryType(entry.Type),
			OriginOrder:      batch.StartOriginOrder + int64(idx),
			PayloadJson:      payload,
			OriginThreadID:   thread.ID,
			OriginRunID:      sql.NullString{},
			OriginSessionID:  nullString(session.ID),
			SessionTimestamp: timestamp,
		})
		if err != nil {
			if isUniqueConstraintError(err) {
				skipped++
				continue
			}
			return imported, skipped, err
		}
		imported++
	}
	return imported, skipped, nil
}

func (s *Service) resolveImportThreadFromScan(
	ctx context.Context,
	q *db.Queries,
	session db.AgentSession,
	workspace db.Workspace,
	scan PiSessionScanSummary,
) (db.AgentThread, bool, error) {
	if session.ThreadID.Valid && strings.TrimSpace(session.ThreadID.String) != "" {
		thread, err := q.GetAgentThread(ctx, session.ThreadID.String)
		if err != nil {
			return db.AgentThread{}, false, err
		}
		if !session.ImportedHeadEntryID.Valid || !thread.HeadEntryID.Valid ||
			thread.HeadEntryID.String == session.ImportedHeadEntryID.String {
			return thread, false, nil
		}
		return s.createImportedThreadFromScan(
			ctx,
			q,
			workspace,
			scan,
			thread.LineageID,
			thread.ID,
			true,
		)
	}
	return s.createImportedThreadFromScan(
		ctx,
		q,
		workspace,
		scan,
		uuid.NewString(),
		"",
		false,
	)
}

func (s *Service) resolveImportThread(
	ctx context.Context,
	q *db.Queries,
	session db.AgentSession,
	workspace db.Workspace,
	doc PiSessionDocument,
) (db.AgentThread, bool, error) {
	if session.ThreadID.Valid && strings.TrimSpace(session.ThreadID.String) != "" {
		thread, err := q.GetAgentThread(ctx, session.ThreadID.String)
		if err != nil {
			return db.AgentThread{}, false, err
		}
		if !session.ImportedHeadEntryID.Valid || !thread.HeadEntryID.Valid ||
			thread.HeadEntryID.String == session.ImportedHeadEntryID.String {
			return thread, false, nil
		}
		return s.createImportedThread(
			ctx,
			q,
			workspace,
			doc,
			thread.LineageID,
			thread.ID,
			true,
		)
	}
	return s.createImportedThread(ctx, q, workspace, doc, uuid.NewString(), "", false)
}

func (s *Service) validatePiSessionPath(path string) (string, error) {
	resolved, err := resolveWorkspacePath(path)
	if err != nil {
		return "", err
	}
	root, err := resolveWorkspacePath(s.piSessionsDir)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(resolved, root) {
		return "", fmt.Errorf(
			"session path %q is outside Pi sessions dir %q",
			resolved,
			root,
		)
	}
	if filepath.Ext(resolved) != ".jsonl" {
		return "", fmt.Errorf("session path must be a .jsonl file")
	}
	return resolved, nil
}

func (s *Service) ensureImportSession(
	ctx context.Context,
	q *db.Queries,
	sessionPath string,
	source AgentSessionSource,
	header PiSessionHeader,
	status, workspaceID, inferredPlanDir, lastError, metadata string,
	userEmail string,
) (db.AgentSession, bool, error) {
	existing, err := q.GetAgentSessionByPath(ctx, nullString(sessionPath))
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.AgentSession{}, false, err
	}
	session, err := q.CreateAgentSession(ctx, db.CreateAgentSessionParams{
		ID:                  uuid.NewString(),
		WorkspaceID:         nullString(workspaceID),
		ThreadID:            sql.NullString{},
		UserEmail:           normalizeSessionOwnerEmail(userEmail),
		Source:              string(source),
		SessionPath:         nullString(sessionPath),
		SessionID:           nullString(header.ID),
		ParentSessionID:     nullString(header.ParentSession),
		Cwd:                 nullString(header.Cwd),
		Status:              status,
		InferredWorkspaceID: nullString(workspaceID),
		InferredPlanDir:     nullString(inferredPlanDir),
		ImportedHeadEntryID: sql.NullString{},
		LastError:           nullString(lastError),
		MetadataJson:        nullString(metadata),
	})
	return session, false, err
}

func normalizeSessionOwnerEmail(userEmail string) sql.NullString {
	return nullString(strings.TrimSpace(userEmail))
}

func (s *Service) validateImportSessionReuse(
	ctx context.Context,
	q *db.Queries,
	session db.AgentSession,
	workspace db.Workspace,
	workspaceOK bool,
	userEmail string,
) error {
	_ = strings.TrimSpace(userEmail)

	hasWorkspace := session.WorkspaceID.Valid &&
		strings.TrimSpace(session.WorkspaceID.String) != ""
	hasThread := session.ThreadID.Valid &&
		strings.TrimSpace(session.ThreadID.String) != ""

	if !session.UserEmail.Valid && !hasWorkspace && !hasThread {
		return errors.New("pi session has no owner and cannot be reused")
	}

	if !workspaceOK && (hasWorkspace || hasThread) {
		return errors.New("pi session is already assigned to a workspace")
	}

	if hasWorkspace {
		existingWorkspace, err := q.GetWorkspace(ctx, session.WorkspaceID.String)
		if err != nil {
			return err
		}
		if workspaceOK && existingWorkspace.ID != workspace.ID {
			return errors.New("pi session is already imported for a different workspace")
		}
	}

	if !hasThread {
		return nil
	}
	thread, err := q.GetAgentThread(ctx, session.ThreadID.String)
	if err != nil {
		return err
	}
	primaryWorkspaceID := ""
	if thread.WorkspaceID.Valid {
		primaryWorkspaceID = strings.TrimSpace(thread.WorkspaceID.String)
	} else {
		rows, err := q.ListThreadWorkspaceAssociations(ctx, thread.ID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.IsPrimary == 1 {
				primaryWorkspaceID = strings.TrimSpace(row.WorkspaceID)
				break
			}
		}
	}
	if primaryWorkspaceID == "" {
		if workspaceOK {
			return errors.New(
				"pi session thread is not attached to the resolved workspace",
			)
		}
		return nil
	}
	if workspaceOK && primaryWorkspaceID != workspace.ID {
		return errors.New("pi session thread belongs to a different workspace")
	}
	if hasWorkspace && primaryWorkspaceID != session.WorkspaceID.String {
		return errors.New("pi session workspace does not match its thread")
	}
	return nil
}

func (s *Service) validateExplicitImportWorkspaceContext(
	ctx context.Context,
	q *db.Queries,
	input SessionImportInput,
) error {
	workspaceID := strings.TrimSpace(input.ExplicitWorkspaceID)
	workspaceDir := strings.TrimSpace(input.ExplicitWorkspaceDir)
	if workspaceID == "" && workspaceDir == "" {
		return nil
	}

	var workspace db.Workspace
	workspaceOK := false
	if workspaceID != "" {
		var err error
		workspace, err = q.GetWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		workspaceOK = true
	}

	if workspaceDir == "" {
		return nil
	}
	root, err := s.validateSharedWorkspaceDir(workspaceDir)
	if err != nil {
		return err
	}
	if workspaceOK && !sameFilesystemPath(root, workspace.RootDocPath) {
		return errors.New("explicit workspace_dir does not match workspace artifact root")
	}
	return nil
}

func (s *Service) validateSharedWorkspaceDir(workspaceDir string) (string, error) {
	return ValidateWorkspaceRootDocPath(workspaceDir, s.thoughtsRoot, "")
}

func (s *Service) resolveImportWorkspaceFromScan(
	ctx context.Context,
	q *db.Queries,
	input SessionImportInput,
	scan PiSessionScanSummary,
) (db.Workspace, bool, error) {
	return s.resolveImportWorkspace(
		ctx,
		q,
		input,
		PiSessionDocument{Header: scan.Header},
		scan.Inference,
	)
}

func (s *Service) resolveImportWorkspace(
	ctx context.Context,
	q *db.Queries,
	input SessionImportInput,
	doc PiSessionDocument,
	inference WorkspaceInferenceResult,
) (db.Workspace, bool, error) {
	if inference.Status == "none" || inference.Status == "ambiguous" {
		return db.Workspace{}, false, nil
	}
	if inference.WorkspaceID != "" {
		workspace, err := q.GetWorkspace(ctx, inference.WorkspaceID)
		if err != nil {
			return db.Workspace{}, false, err
		}
		if strings.TrimSpace(input.ExplicitWorkspaceDir) != "" {
			root, err := s.validateSharedWorkspaceDir(input.ExplicitWorkspaceDir)
			if err != nil {
				return db.Workspace{}, false, err
			}
			if !sameFilesystemPath(root, workspace.RootDocPath) {
				return db.Workspace{}, false, fmt.Errorf(
					"explicit workspace_dir does not match workspace artifact root",
				)
			}
		}
		return workspace, true, nil
	}

	root := inference.PlanDir
	if root == "" {
		root = input.ExplicitWorkspaceDir
	}
	validatedRoot, err := s.validateSharedWorkspaceDir(root)
	if err != nil {
		return db.Workspace{}, false, err
	}

	workspace, err := q.FindWorkspaceByRootDocPath(ctx, validatedRoot)
	if err == nil {
		return workspace, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.Workspace{}, false, err
	}

	owner := provenanceUserForWorkspaceRoot(
		validatedRoot,
		s.thoughtsRoot,
		input.UserEmail,
	)
	if owner == "" {
		return db.Workspace{}, false, fmt.Errorf(
			"could not infer workspace user from %q",
			validatedRoot,
		)
	}
	workspaceID, err := NewWorkspaceID()
	if err != nil {
		return db.Workspace{}, false, err
	}
	workspace, err = q.CreateWorkspace(ctx, db.CreateWorkspaceParams{
		ID:                workspaceID,
		UserEmail:         owner,
		Title:             validateWorkspaceTitle(filepath.Base(validatedRoot)),
		RootDocPath:       validatedRoot,
		Cwd:               nullString(doc.Header.Cwd),
		WorkflowType:      string(workspaceTypeForPlanDir(validatedRoot)),
		WorkflowStateJson: sql.NullString{},
		Source:            string(WorkspaceSourceImported),
		SelectedThreadID:  sql.NullString{},
		SelectedDocPath:   sql.NullString{},
	})
	return workspace, err == nil, err
}

func (s *Service) createImportedThreadFromScan(
	ctx context.Context,
	q *db.Queries,
	workspace db.Workspace,
	scan PiSessionScanSummary,
	lineageID, parentThreadID string,
	diverged bool,
) (db.AgentThread, bool, error) {
	title := "Terminal session"
	if text := strings.TrimSpace(scan.FirstUserText); text != "" {
		title = truncateTitle(text)
	} else if scan.Header.ID != "" {
		title = "Terminal session " + scan.Header.ID
	}
	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         workspace.UserEmail,
		Title:             title,
		Cwd:               workspace.RootDocPath,
		LineageID:         lineageID,
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    nullString(parentThreadID),
		ForkedFromEntryID: sql.NullString{},
	})
	return thread, diverged, err
}

func (s *Service) createImportedThread(
	ctx context.Context,
	q *db.Queries,
	workspace db.Workspace,
	doc PiSessionDocument,
	lineageID, parentThreadID string,
	diverged bool,
) (db.AgentThread, bool, error) {
	title := importThreadTitle(doc)
	thread, err := q.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:                uuid.NewString(),
		UserEmail:         workspace.UserEmail,
		Title:             title,
		Cwd:               workspace.RootDocPath,
		LineageID:         lineageID,
		HeadEntryID:       sql.NullString{},
		ParentThreadID:    nullString(parentThreadID),
		ForkedFromEntryID: sql.NullString{},
	})
	return thread, diverged, err
}

func statusForInference(inference WorkspaceInferenceResult) string {
	switch inference.Status {
	case "none":
		return "unassigned"
	case "ambiguous":
		return "ambiguous"
	default:
		return "pending"
	}
}

func maybeWorkspaceID(workspace db.Workspace, ok bool) string {
	if !ok {
		return ""
	}
	return workspace.ID
}

func importMetadataJSON(inference WorkspaceInferenceResult) string {
	payload, err := json.Marshal(inference)
	if err != nil {
		return ""
	}
	return string(payload)
}

func importMetadataJSONWithStats(
	inference WorkspaceInferenceResult,
	stats PiSessionImportStats,
) string {
	return importMetadataJSONPayload(sessionImportMetadata{
		Inference: inference,
		Stats:     stats,
	})
}

func importMetadataJSONWithSummary(
	scan PiSessionScanSummary,
	stats PiSessionImportStats,
) string {
	return importMetadataJSONPayload(sessionImportMetadata{
		Inference:     scan.Inference,
		Stats:         stats,
		Header:        scan.Header,
		FirstUserText: strings.TrimSpace(scan.FirstUserText),
	})
}

func importMetadataJSONPayload(payload sessionImportMetadata) string {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func parsePiTimestamp(raw string) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts, nil
	}
	if ms, err := time.Parse("2006-01-02T15:04:05.000Z", raw); err == nil {
		return ms, nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", raw)
}

func normalizePiEntryType(entryType string) string {
	entryType = strings.TrimSpace(entryType)
	switch entryType {
	case "custom":
		return "custom_message"
	default:
		return entryType
	}
}

func importThreadTitle(doc PiSessionDocument) string {
	for _, entry := range doc.Entries {
		if strings.TrimSpace(entry.Message.Role) == "user" {
			if text := strings.TrimSpace(
				extractContentText(entry.Message.Content),
			); text != "" {
				return truncateTitle(text)
			}
		}
	}
	if doc.Header.ID != "" {
		return "Terminal session " + doc.Header.ID
	}
	return "Terminal session"
}

func workspaceTypeForPlanDir(root string) WorkspaceWorkflowType {
	if strings.Contains(filepath.ToSlash(root), "/plans/") {
		return WorkspaceWorkflowQRSPI
	}
	return WorkspaceWorkflowFreeform
}

func provenanceUserForWorkspaceRoot(root, thoughtsRoot, fallbackUserEmail string) string {
	if owner := userEmailFromPlanDir(root, thoughtsRoot); strings.TrimSpace(owner) != "" {
		return strings.TrimSpace(owner)
	}
	return strings.TrimSpace(fallbackUserEmail)
}

func userEmailFromPlanDir(planDir, thoughtsRoot string) string {
	root, err := resolveWorkspacePath(thoughtsRoot)
	if err != nil {
		return ""
	}
	plan, err := resolveWorkspacePath(planDir)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(root, plan)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) >= 3 && parts[1] == "plans" {
		return parts[0]
	}
	return ""
}

type toolCallRef struct {
	ID        string
	Name      string
	Arguments any
}

func extractToolCalls(content any) []toolCallRef {
	items, ok := content.([]any)
	if !ok {
		return nil
	}
	calls := []toolCallRef{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok || fmt.Sprint(m["type"]) != "toolCall" {
			continue
		}
		calls = append(
			calls,
			toolCallRef{
				ID:        strings.TrimSpace(fmt.Sprint(m["id"])),
				Name:      strings.TrimSpace(fmt.Sprint(m["name"])),
				Arguments: firstPresent(m, "arguments", "input"),
			},
		)
	}
	return calls
}

func firstPresent(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			return value
		}
	}
	return nil
}

func extractPathStrings(values ...any) []string {
	paths := []string{}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case nil:
			return
		case string:
			if looksLikePath(v) {
				paths = append(paths, v)
			}
		case map[string]any:
			for key, child := range v {
				lower := strings.ToLower(key)
				if strings.Contains(lower, "path") || lower == "file" ||
					lower == "filename" {
					if s, ok := child.(string); ok {
						paths = append(paths, s)
						continue
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	for _, value := range values {
		walk(value)
	}
	return paths
}

func looksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	return strings.Contains(value, "/") ||
		strings.Contains(value, string(filepath.Separator)) ||
		strings.Contains(value, ".")
}

func planDirForTouchedPath(rawPath, thoughtsRoot, cwd string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", false
	}
	if !filepath.IsAbs(rawPath) && strings.TrimSpace(cwd) != "" {
		rawPath = filepath.Join(cwd, rawPath)
	}
	resolved, err := resolveWorkspacePath(rawPath)
	if err != nil {
		return "", false
	}
	root, err := resolveWorkspacePath(thoughtsRoot)
	if err != nil || !pathWithinRoot(resolved, root) {
		return "", false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 || parts[1] != "plans" {
		return "", false
	}
	return filepath.Join(root, parts[0], parts[1], parts[2]), true
}

func sessionImportEventKey(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(hash[:])
}

func copyLimited(r io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(r, limit)
	return io.ReadAll(limited)
}

func (s *Service) AdoptImportedQRSPIState(
	ctx context.Context,
	workspaceID string,
	threadID string,
	headEntryID string,
) (bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	threadID = strings.TrimSpace(threadID)
	headEntryID = strings.TrimSpace(headEntryID)
	if workspaceID == "" || threadID == "" || headEntryID == "" {
		return false, nil
	}
	workspace, err := s.queries.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	if WorkspaceWorkflowType(strings.TrimSpace(workspace.WorkflowType)) != WorkspaceWorkflowQRSPI {
		return false, nil
	}
	adapter, ok := s.workflowService.(*agentchatworkflows.Service)
	if !ok || adapter == nil || adapter.Definitions == nil || adapter.Store == nil {
		return false, nil
	}
	def, ok := adapter.Definitions.Get(qrspi.AgentChatWorkflowType)
	if !ok {
		return false, nil
	}
	state, err := adapter.Store.LoadWorkspaceState(ctx, workspaceID)
	if err != nil {
		policy, policyErr := json.Marshal(qrspi.DefaultPolicy())
		if policyErr != nil {
			return false, policyErr
		}
		state, err = wruntime.InitialState(def, policy)
		if err != nil {
			return false, err
		}
	}
	currentDef, ok := adapter.Definitions.Get(wruntime.WorkflowID(strings.TrimSpace(state.Type)))
	if !ok {
		return false, nil
	}
	assistant, err := adapter.Store.FinalAssistantText(ctx, threadID, headEntryID)
	if err != nil {
		return false, err
	}
	parseCtx := wruntime.ParseContext{
		WorkflowType: strings.TrimSpace(state.Type),
		ThreadID:     threadID,
		HeadEntryID:  headEntryID,
	}
	parsed, err := currentDef.ResultParser.Parse(assistant, parseCtx)
	if err != nil {
		return false, nil
	}
	workflowResult, err := currentDef.ResultConverter.ToWorkflowResult(parsed, parseCtx)
	if err != nil {
		return false, err
	}
	_, applied, err := adapter.ApplyExternalWorkflowResult(
		ctx,
		agentchatworkflows.ExternalWorkflowResultInput{
			WorkspaceID: workspaceID,
			ThreadID:    threadID,
			HeadEntryID: headEntryID,
			State:       state,
			Result:      workflowResult,
		},
	)
	if err != nil {
		return false, err
	}
	if !applied {
		return false, nil
	}
	_ = s.queries.UpdateWorkspaceSelectedThread(
		ctx,
		db.UpdateWorkspaceSelectedThreadParams{
			ID:               workspaceID,
			SelectedThreadID: nullString(threadID),
		},
	)
	return true, nil
}
