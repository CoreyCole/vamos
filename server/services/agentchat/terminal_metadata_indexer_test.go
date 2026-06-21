package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTerminalMetadataEventValidatesRequiredFields(t *testing.T) {
	validTime := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := ParseTerminalMetadataEvent([]byte(fmt.Sprintf(`{"schema_version":1,"event_id":"event-1","event_type":"session_start","event_time":%q}`, validTime))); err != nil {
		t.Fatalf("ParseTerminalMetadataEvent(valid) error = %v", err)
	}

	cases := map[string]string{
		"schema":     fmt.Sprintf(`{"schema_version":2,"event_id":"event-1","event_type":"session_start","event_time":%q}`, validTime),
		"event_id":   fmt.Sprintf(`{"schema_version":1,"event_type":"session_start","event_time":%q}`, validTime),
		"event_type": fmt.Sprintf(`{"schema_version":1,"event_id":"event-1","event_time":%q}`, validTime),
		"event_time": `{"schema_version":1,"event_id":"event-1","event_type":"session_start"}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseTerminalMetadataEvent([]byte(payload)); err == nil {
				t.Fatalf("ParseTerminalMetadataEvent(%s) error = nil, want validation error", name)
			}
		})
	}
}

func TestIndexTerminalMetadataUpsertsSessionAndAdvancesCursor(t *testing.T) {
	service := newTestAgentChatService(t)
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")
	writeTerminalMetadataEvents(t, logPath,
		terminalMetadataEventLine("event-session", "session_start", sessionPath, ""),
		terminalMetadataEventLine("event-result", "qrspi_result", sessionPath, `,"qrspi":{"stage":"outline","status":"complete","outcome":"complete","artifact":"thoughts/agent/plans/2026-06-02_plan/outline.md"}`),
	)

	result, err := service.IndexTerminalMetadata(t.Context(), TerminalMetadataIndexInput{EventLogPath: logPath})
	if err != nil {
		t.Fatalf("IndexTerminalMetadata() error = %v", err)
	}
	if result.EventsRead != 2 || result.SessionsUpserted != 2 || result.QRSPIProjected != 1 || result.Failed != 0 || !result.CursorAdvanced || !result.Changed {
		t.Fatalf("IndexTerminalMetadata() = %+v, want two events/session upserts and one projection", result)
	}

	session, err := service.queries.GetAgentSessionByPath(t.Context(), nullableString(sessionPath))
	if err != nil {
		t.Fatalf("GetAgentSessionByPath() error = %v", err)
	}
	if session.IdentityKind != "global_pi" || session.ProjectionState != "needs_hydration" {
		t.Fatalf("session identity/state = %s/%s, want global_pi/needs_hydration", session.IdentityKind, session.ProjectionState)
	}
	if !session.PlanDir.Valid || session.PlanDir.String != "agent/plans/2026-06-02_plan" {
		t.Fatalf("session.PlanDir = %#v, want relative plan dir", session.PlanDir)
	}

	projections, err := service.queries.ListPendingQRSPIProjections(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListPendingQRSPIProjections() error = %v", err)
	}
	if len(projections) != 1 {
		t.Fatalf("pending projections = %d, want 1", len(projections))
	}
	if projections[0].SourceEventID != "event-result" || projections[0].PlanDir != "thoughts/agent/plans/2026-06-02_plan" {
		t.Fatalf("projection source/plan = %s/%s", projections[0].SourceEventID, projections[0].PlanDir)
	}

	cursor, err := service.queries.GetPiMetadataCursor(t.Context(), logPath)
	if err != nil {
		t.Fatalf("GetPiMetadataCursor() error = %v", err)
	}
	if cursor.ByteOffset <= 0 || !cursor.LastEventID.Valid || cursor.LastEventID.String != "event-result" {
		t.Fatalf("cursor = %+v, want advanced to event-result", cursor)
	}

	second, err := service.IndexTerminalMetadata(t.Context(), TerminalMetadataIndexInput{EventLogPath: logPath})
	if err != nil {
		t.Fatalf("IndexTerminalMetadata(second) error = %v", err)
	}
	if second.EventsRead != 0 || second.SessionsUpserted != 0 || second.QRSPIProjected != 0 || second.CursorAdvanced || second.Changed {
		t.Fatalf("IndexTerminalMetadata(second) = %+v, want no-op", second)
	}
}

func TestIndexTerminalMetadataSkipsMalformedLineAndAdvancesPastIt(t *testing.T) {
	service := newTestAgentChatService(t)
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")
	writeTerminalMetadataEvents(t, logPath,
		`{"schema_version":1,"event_id":"bad"}`,
		terminalMetadataEventLine("event-session", "session_start", sessionPath, ""),
	)

	result, err := service.IndexTerminalMetadata(t.Context(), TerminalMetadataIndexInput{EventLogPath: logPath})
	if err != nil {
		t.Fatalf("IndexTerminalMetadata() error = %v", err)
	}
	if result.EventsRead != 1 || result.Failed != 1 || !result.CursorAdvanced {
		t.Fatalf("IndexTerminalMetadata() = %+v, want one valid, one failed, advanced cursor", result)
	}
	cursor, err := service.queries.GetPiMetadataCursor(t.Context(), logPath)
	if err != nil {
		t.Fatalf("GetPiMetadataCursor() error = %v", err)
	}
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Stat(logPath) error = %v", err)
	}
	if cursor.ByteOffset != info.Size() {
		t.Fatalf("cursor offset = %d, want EOF %d", cursor.ByteOffset, info.Size())
	}
}

func TestIndexTerminalMetadataDoesNotAdvanceCursorOnTransactionError(t *testing.T) {
	service := newTestAgentChatService(t)
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	sessionPath := filepath.Join(t.TempDir(), "session.jsonl")
	writeTerminalMetadataEvents(t, logPath, terminalMetadataEventLine("event-session", "session_start", sessionPath, ""))
	service.terminalMetadataBeforeCommitForTest = func() error { return errors.New("forced transaction failure") }

	result, err := service.IndexTerminalMetadata(t.Context(), TerminalMetadataIndexInput{EventLogPath: logPath})
	if err == nil || !strings.Contains(err.Error(), "forced transaction failure") {
		t.Fatalf("IndexTerminalMetadata() error = %v, want forced transaction failure", err)
	}
	if result.CursorAdvanced {
		t.Fatalf("IndexTerminalMetadata() = %+v, want no cursor advancement", result)
	}
	if _, err := service.queries.GetPiMetadataCursor(context.Background(), logPath); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetPiMetadataCursor() error = %v, want sql.ErrNoRows", err)
	}
	if _, err := service.queries.GetAgentSessionByPath(context.Background(), nullableString(sessionPath)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetAgentSessionByPath() error = %v, want rollback", err)
	}
}

func TestIndexTerminalMetadataKeepsExistingCursorOnBusyCommitError(t *testing.T) {
	service := newTestAgentChatService(t)
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	firstSessionPath := filepath.Join(t.TempDir(), "session-1.jsonl")
	writeTerminalMetadataEvents(t, logPath, terminalMetadataEventLine("event-session-1", "session_start", firstSessionPath, ""))

	first, err := service.IndexTerminalMetadata(t.Context(), TerminalMetadataIndexInput{EventLogPath: logPath})
	if err != nil {
		t.Fatalf("IndexTerminalMetadata(first) error = %v", err)
	}
	if !first.CursorAdvanced {
		t.Fatalf("IndexTerminalMetadata(first) = %+v, want cursor advanced", first)
	}
	oldCursor, err := service.queries.GetPiMetadataCursor(t.Context(), logPath)
	if err != nil {
		t.Fatalf("GetPiMetadataCursor(old) error = %v", err)
	}

	secondSessionPath := filepath.Join(t.TempDir(), "session-2.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile(events) error = %v", err)
	}
	if _, err := fmt.Fprintln(f, terminalMetadataEventLine("event-session-2", "session_start", secondSessionPath, "")); err != nil {
		t.Fatalf("append event error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close(events) error = %v", err)
	}

	service.terminalMetadataBeforeCommitForTest = func() error { return errors.New("database is locked") }
	result, err := service.IndexTerminalMetadata(t.Context(), TerminalMetadataIndexInput{EventLogPath: logPath})
	if err == nil || !strings.Contains(err.Error(), "database is locked") {
		t.Fatalf("IndexTerminalMetadata(second) error = %v, want database is locked", err)
	}
	if result.CursorAdvanced {
		t.Fatalf("IndexTerminalMetadata(second) = %+v, want no cursor advancement", result)
	}
	newCursor, err := service.queries.GetPiMetadataCursor(t.Context(), logPath)
	if err != nil {
		t.Fatalf("GetPiMetadataCursor(new) error = %v", err)
	}
	if newCursor.ByteOffset != oldCursor.ByteOffset || newCursor.LastEventID.String != oldCursor.LastEventID.String {
		t.Fatalf("cursor = %+v, want unchanged from %+v", newCursor, oldCursor)
	}
	if _, err := service.queries.GetAgentSessionByPath(context.Background(), nullableString(secondSessionPath)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetAgentSessionByPath(second) error = %v, want rollback", err)
	}
}

func writeTerminalMetadataEvents(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(events) error = %v", err)
	}
}

func terminalMetadataEventLine(eventID, eventType, sessionPath, extra string) string {
	return fmt.Sprintf(`{"schema_version":1,"event_id":%q,"event_type":%q,"event_time":"2026-06-20T12:00:00Z","pi":{"session_id":"session-1","session_file":%q,"cwd":"/tmp/project/thoughts/agent/plans/2026-06-02_plan"},"plan":{"plan_dir":"thoughts/agent/plans/2026-06-02_plan"}%s}`,
		eventID,
		eventType,
		sessionPath,
		extra,
	)
}
