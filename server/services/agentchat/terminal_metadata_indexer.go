package agentchat

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	defaultTerminalMetadataMaxEvents   = 500
	defaultTerminalMetadataMaxBytes    = int64(1 << 20)
	defaultTerminalMetadataMaxDuration = 10 * time.Second
)

func DefaultPiMetadataLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "vamos", "pi-sessions", "events.jsonl")
}

func (s *Service) IndexTerminalMetadata(
	ctx context.Context,
	input TerminalMetadataIndexInput,
) (TerminalMetadataIndexResult, error) {
	if s == nil || s.db == nil || s.queries == nil {
		return TerminalMetadataIndexResult{}, errors.New("terminal metadata indexer requires service database")
	}
	input = normalizeTerminalMetadataIndexInput(input)
	if strings.TrimSpace(input.EventLogPath) == "" {
		return TerminalMetadataIndexResult{}, nil
	}

	info, err := os.Stat(input.EventLogPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TerminalMetadataIndexResult{}, nil
		}
		return TerminalMetadataIndexResult{}, err
	}
	if info.IsDir() {
		return TerminalMetadataIndexResult{}, fmt.Errorf("terminal metadata log path %q is a directory", input.EventLogPath)
	}

	cursorOffset, err := s.piMetadataCursorOffset(ctx, input.EventLogPath, info.Size())
	if err != nil {
		return TerminalMetadataIndexResult{}, err
	}

	file, err := os.Open(input.EventLogPath)
	if err != nil {
		return TerminalMetadataIndexResult{}, err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Seek(cursorOffset, io.SeekStart); err != nil {
		return TerminalMetadataIndexResult{}, err
	}

	batch, result, err := readTerminalMetadataBatch(ctx, file, cursorOffset, input)
	if err != nil {
		return result, err
	}
	if len(batch.projections) == 0 && batch.offset == cursorOffset {
		return result, nil
	}

	identity := terminalMetadataSourceIdentity(info)
	if err := s.commitTerminalMetadataBatch(ctx, input.EventLogPath, identity, batch); err != nil {
		return result, err
	}
	result.CursorAdvanced = batch.offset != cursorOffset
	result.Changed = result.CursorAdvanced || result.SessionsUpserted > 0 || result.QRSPIProjected > 0
	return result, nil
}

func normalizeTerminalMetadataIndexInput(input TerminalMetadataIndexInput) TerminalMetadataIndexInput {
	input.EventLogPath = strings.TrimSpace(input.EventLogPath)
	if input.EventLogPath == "" {
		input.EventLogPath = DefaultPiMetadataLogPath()
	}
	if input.MaxEvents <= 0 {
		input.MaxEvents = defaultTerminalMetadataMaxEvents
	}
	if input.MaxBytes <= 0 {
		input.MaxBytes = defaultTerminalMetadataMaxBytes
	}
	if input.MaxDuration <= 0 {
		input.MaxDuration = defaultTerminalMetadataMaxDuration
	}
	return input
}

func (s *Service) piMetadataCursorOffset(
	ctx context.Context,
	sourcePath string,
	fileSize int64,
) (int64, error) {
	cursor, err := s.queries.GetPiMetadataCursor(ctx, sourcePath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	if cursor.ByteOffset > fileSize {
		markErr := fmt.Sprintf("cursor offset %d is past log size %d", cursor.ByteOffset, fileSize)
		if err := s.queries.MarkPiMetadataCursorFailed(ctx, db.MarkPiMetadataCursorFailedParams{
			SourcePath:     sourcePath,
			SourceIdentity: cursor.SourceIdentity,
			ByteOffset:     cursor.ByteOffset,
			LastError:      nullString(markErr),
		}); err != nil {
			return 0, err
		}
		return 0, errors.New(markErr)
	}
	return cursor.ByteOffset, nil
}

type terminalMetadataBatch struct {
	projections []TerminalMetadataProjection
	offset      int64
	lastEventID string
	lastEventAt sql.NullTime
}

func readTerminalMetadataBatch(
	ctx context.Context,
	reader io.Reader,
	startOffset int64,
	input TerminalMetadataIndexInput,
) (terminalMetadataBatch, TerminalMetadataIndexResult, error) {
	var batch terminalMetadataBatch
	batch.offset = startOffset
	var result TerminalMetadataIndexResult
	started := time.Now()
	buf := bufio.NewReader(reader)
	bytesRead := int64(0)

	for result.EventsRead < input.MaxEvents && bytesRead < input.MaxBytes {
		if err := ctx.Err(); err != nil {
			return batch, result, err
		}
		if time.Since(started) >= input.MaxDuration {
			break
		}

		line, err := buf.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return batch, result, err
		}
		if input.MaxBytes > 0 && bytesRead+int64(len(line)) > input.MaxBytes {
			break
		}
		bytesRead += int64(len(line))
		batch.offset += int64(len(line))

		event, parseErr := ParseTerminalMetadataEvent(line)
		if parseErr != nil {
			result.Failed++
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		projection, normalizeErr := NormalizeTerminalMetadataEvent(event)
		if normalizeErr != nil {
			result.Failed++
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		result.EventsRead++
		batch.projections = append(batch.projections, projection)
		batch.lastEventID = strings.TrimSpace(event.EventID)
		batch.lastEventAt = sql.NullTime{Time: event.EventTime, Valid: true}
		if projection.ArtifactPath != "" || projection.ExternalSessionID != "" {
			result.SessionsUpserted++
		}
		if projection.QRSPIResult != nil {
			result.QRSPIProjected++
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}

	return batch, result, nil
}

func (s *Service) commitTerminalMetadataBatch(
	ctx context.Context,
	sourcePath string,
	sourceIdentity string,
	batch terminalMetadataBatch,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)

	for _, projection := range batch.projections {
		projection = enrichTerminalMetadataProjectionFromFile(projection)
		if projection.ArtifactPath != "" || projection.ExternalSessionID != "" {
			if _, err := q.UpsertAgentSessionIndex(ctx, terminalMetadataAgentSessionParams(projection)); err != nil {
				return err
			}
		}
		if projection.QRSPIResult != nil {
			if _, err := q.UpsertQRSPIProjectionPending(ctx, terminalMetadataQRSPIParams(*projection.QRSPIResult)); err != nil {
				return err
			}
		}
	}

	if s.terminalMetadataBeforeCommitForTest != nil {
		if err := s.terminalMetadataBeforeCommitForTest(); err != nil {
			return err
		}
	}
	if _, err := q.UpsertPiMetadataCursorAdvanced(ctx, db.UpsertPiMetadataCursorAdvancedParams{
		SourcePath:     sourcePath,
		SourceIdentity: nullString(sourceIdentity),
		ByteOffset:     batch.offset,
		LastEventID:    nullString(batch.lastEventID),
		LastEventTime:  batch.lastEventAt,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func terminalMetadataAgentSessionParams(
	projection TerminalMetadataProjection,
) db.UpsertAgentSessionIndexParams {
	return db.UpsertAgentSessionIndexParams{
		ID:                     uuid.NewString(),
		IdentityKind:           "global_pi",
		ArtifactPath:           nullableString(projection.ArtifactPath),
		PlanDir:                nullableString(agentSessionPlanDirFromMetadata(projection.PlanDir)),
		ParentPlanDir:          sql.NullString{},
		SourceReviewDir:        sql.NullString{},
		Agent:                  defaultAgentSessionAgent,
		ExternalSessionID:      nullableString(projection.ExternalSessionID),
		ParentSessionID:        sql.NullString{},
		Cwd:                    nullableString(projection.Cwd),
		WorkflowID:             sql.NullString{},
		WorkflowNodeID:         nullableString(projection.WorkflowNodeID),
		ContinuedFromSessionID: sql.NullString{},
		ForkedFromSessionID:    sql.NullString{},
		FileSize:               projection.FileSize,
		FileMtime:              nullTime(projection.FileMtime),
		FileHash:               sql.NullString{},
		LastIndexedOffset:      projection.LastIndexedOffset,
		ProjectionState:        "needs_hydration",
		ProjectedThreadID:      sql.NullString{},
		IndexedByUserEmail:     sql.NullString{},
		AttachedWorkspaceID:    nullableString(projection.WorkspaceID),
		LastError:              sql.NullString{},
		MetadataJson:           nullableString(projection.MetadataJSON),
	}
}

func terminalMetadataQRSPIParams(
	projection QRSPIResultProjection,
) db.UpsertQRSPIProjectionPendingParams {
	return db.UpsertQRSPIProjectionPendingParams{
		ID:                  projection.ID,
		SourceEventID:       projection.SourceEventID,
		SessionID:           nullableString(projection.SessionID),
		SessionArtifactPath: nullableString(projection.SessionArtifactPath),
		PlanDir:             projection.PlanDir,
		WorkflowNodeID:      nullableString(projection.WorkflowNodeID),
		Stage:               nullableString(projection.Stage),
		Status:              nullableString(projection.Status),
		Outcome:             nullableString(projection.Outcome),
		Artifact:            nullableString(projection.Artifact),
		ResultJson:          projection.ResultJSON,
		EventTime:           projection.EventTime,
	}
}

func enrichTerminalMetadataProjectionFromFile(
	projection TerminalMetadataProjection,
) TerminalMetadataProjection {
	if strings.TrimSpace(projection.ArtifactPath) == "" {
		return projection
	}
	info, err := os.Stat(projection.ArtifactPath)
	if err != nil || info.IsDir() {
		return projection
	}
	projection.FileSize = info.Size()
	projection.FileMtime = info.ModTime()
	if projection.LastIndexedOffset == 0 {
		projection.LastIndexedOffset = info.Size()
	}
	return projection
}

func terminalMetadataSourceIdentity(info os.FileInfo) string {
	return fmt.Sprintf("size=%d;mtime=%s", info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano))
}

func nullTime(value time.Time) sql.NullTime {
	return sql.NullTime{Time: value, Valid: !value.IsZero()}
}

func agentSessionPlanDirFromMetadata(planDir string) string {
	planDir = strings.TrimSpace(planDir)
	if strings.HasPrefix(planDir, "thoughts/") {
		return strings.TrimPrefix(planDir, "thoughts/")
	}
	return planDir
}
