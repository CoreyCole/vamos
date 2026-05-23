package agentchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/db"
)

func TestParsePiSessionJSONLRejectsMalformedAndMissingParent(t *testing.T) {
	service := newTestAgentChatService(t)
	path := filepath.Join(service.piSessionsDir, "bad.jsonl")
	writePiSessionFile(
		t,
		path,
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"e1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"message","id":"e2","parentId":"missing","timestamp":"2026-04-30T12:00:02Z","message":{"role":"assistant","content":"no"}}`,
	)
	if _, err := ParsePiSessionJSONL(
		path,
	); err == nil ||
		!strings.Contains(err.Error(), "missing parent") {
		t.Fatalf("ParsePiSessionJSONL() error = %v, want missing parent", err)
	}
}

func TestImportPiSessionAutoAdoptsSingleTouchedPlan(t *testing.T) {
	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_demo",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "session.jsonl")
	artifactPath := filepath.Join(planDir, "design.md")
	writePiSessionFile(t, sessionPath, piImportFixtureLines(artifactPath)...)

	result, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath, UserEmail: "viewer@example.com"},
	)
	if err != nil {
		t.Fatalf("ImportPiSession() error = %v", err)
	}
	if result.Status != "imported" || result.WorkspaceID == "" || result.ThreadID == "" ||
		result.ImportedHeadEntry != "tool-1" {
		t.Fatalf("result = %+v, want imported workspace/thread/head", result)
	}
	if result.Stats.BatchCount < 1 || result.Stats.EntriesRead != 3 ||
		result.Stats.EntriesImported != 3 || result.Stats.EntriesSkipped != 0 {
		t.Fatalf("result stats = %+v, want first import counts", result.Stats)
	}
	workspace, err := service.queries.GetWorkspace(
		context.Background(),
		result.WorkspaceID,
	)
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	if workspace.UserEmail != "user@example.com" || workspace.RootDocPath != planDir {
		t.Fatalf("workspace = %+v, want user/root", workspace)
	}

	reimportByOtherViewer, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{
			SessionPath: sessionPath,
			UserEmail:   "other-viewer@example.com",
		},
	)
	if err != nil {
		t.Fatalf("cross-viewer reimport ImportPiSession() error = %v", err)
	}
	if reimportByOtherViewer.WorkspaceID != result.WorkspaceID ||
		reimportByOtherViewer.ThreadID != result.ThreadID {
		t.Fatalf(
			"cross-viewer reimport = %+v, want workspace/thread %s/%s",
			reimportByOtherViewer,
			result.WorkspaceID,
			result.ThreadID,
		)
	}

	reimport, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("reimport ImportPiSession() error = %v", err)
	}
	if reimport.ThreadID != result.ThreadID ||
		reimport.ImportedHeadEntry != result.ImportedHeadEntry {
		t.Fatalf("reimport = %+v, want same thread/head as %+v", reimport, result)
	}
	if reimport.Stats.EntriesRead != 3 || reimport.Stats.EntriesImported != 0 ||
		reimport.Stats.EntriesSkipped != 3 {
		t.Fatalf("reimport stats = %+v, want duplicate skips", reimport.Stats)
	}
}

func TestImportPiSessionDivergenceCreatesSiblingThread(t *testing.T) {
	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_demo",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "session.jsonl")
	writePiSessionFile(
		t,
		sessionPath,
		piImportFixtureLines(filepath.Join(planDir, "design.md"))...)

	first, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("ImportPiSession() error = %v", err)
	}
	if err := service.queries.UpdateAgentThreadHead(
		context.Background(),
		db.UpdateAgentThreadHeadParams{
			ID:          first.ThreadID,
			HeadEntryID: nullString("web-head"),
		},
	); err != nil {
		t.Fatalf("UpdateAgentThreadHead() error = %v", err)
	}
	second, err := service.ImportPiSession(
		context.Background(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("second ImportPiSession() error = %v", err)
	}
	if !second.Diverged || second.Status != "diverged" ||
		second.ThreadID == first.ThreadID {
		t.Fatalf("second = %+v, want diverged sibling", second)
	}
}

func TestImportPiSessionLargeSessionUsesMultipleBatches(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_large",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "large.jsonl")
	entryCount := defaultPiSessionImportBatchSize + 5
	writePiSessionFile(
		t,
		sessionPath,
		largePiImportFixtureLines(filepath.Join(planDir, "design.md"), entryCount, -1)...,
	)

	result, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("ImportPiSession() error = %v", err)
	}
	if result.Status != "imported" ||
		result.ImportedHeadEntry != piEntryID(entryCount-1) {
		t.Fatalf("result = %+v, want imported final head", result)
	}
	if result.Stats.BatchCount != 2 || result.Stats.EntriesRead != entryCount ||
		result.Stats.EntriesImported != entryCount || result.Stats.EntriesSkipped != 0 {
		t.Fatalf("stats = %+v, want two full import batches", result.Stats)
	}
}

func TestImportPiSessionPartialBatchFailureCanRetry(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	planDir := filepath.Join(
		service.thoughtsRoot,
		"user@example.com",
		"plans",
		"2026-04-30_retry",
	)
	if err := ensureDir(planDir); err != nil {
		t.Fatalf("ensureDir(planDir): %v", err)
	}
	sessionPath := filepath.Join(service.piSessionsDir, "retry.jsonl")
	entryCount := defaultPiSessionImportBatchSize + 5
	badIndex := defaultPiSessionImportBatchSize + 1
	writePiSessionFile(
		t,
		sessionPath,
		largePiImportFixtureLines(
			filepath.Join(planDir, "design.md"),
			entryCount,
			badIndex,
		)...,
	)

	failed, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err == nil || !strings.Contains(err.Error(), "invalid timestamp") {
		t.Fatalf("ImportPiSession() error = %v, want invalid timestamp", err)
	}
	if failed.Status != "failed" || failed.Stats.FailedLine == 0 ||
		failed.Stats.EntriesImported != defaultPiSessionImportBatchSize {
		t.Fatalf("failed result = %+v, want partial failure stats", failed)
	}
	session, err := service.queries.GetAgentSession(
		t.Context(),
		failed.SessionID,
	)
	if err != nil {
		t.Fatalf("GetAgentSession() error = %v", err)
	}
	if session.Status != "failed" || session.LastImportedAt.Valid ||
		!session.LastError.Valid {
		t.Fatalf("session = %+v, want failed without last_imported_at", session)
	}
	if !strings.Contains(session.MetadataJson.String, `"failed_line"`) {
		t.Fatalf("metadata_json = %s, want failed_line", session.MetadataJson.String)
	}

	writePiSessionFile(
		t,
		sessionPath,
		largePiImportFixtureLines(filepath.Join(planDir, "design.md"), entryCount, -1)...,
	)
	retry, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: sessionPath},
	)
	if err != nil {
		t.Fatalf("retry ImportPiSession() error = %v", err)
	}
	if retry.Status != "imported" ||
		retry.Stats.EntriesSkipped != defaultPiSessionImportBatchSize ||
		retry.Stats.EntriesImported != entryCount-defaultPiSessionImportBatchSize {
		t.Fatalf("retry = %+v, want duplicate skip recovery", retry)
	}
}

func TestImportPiSessionRejectsSymlinkEscapeBeforeScan(t *testing.T) {
	t.Parallel()

	service := newTestAgentChatService(t)
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	writePiSessionFile(
		t,
		outside,
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`not-json`,
	)
	link := filepath.Join(service.piSessionsDir, "escape-import.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := service.ImportPiSession(
		t.Context(),
		SessionImportInput{SessionPath: link},
	); err == nil || !strings.Contains(err.Error(), "outside Pi sessions dir") {
		t.Fatalf("ImportPiSession() error = %v, want path rejection before scan", err)
	}
	var count int
	if err := service.db.QueryRowContext(
		t.Context(),
		`SELECT COUNT(*) FROM agent_sessions`,
	).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Fatalf("agent_sessions count = %d, want no scan/import side effects", count)
	}
}

func TestInternalPiSessionImportRequiresToken(t *testing.T) {
	service := newTestAgentChatService(t)
	handler := NewHandler(service, nil, HandlerOptions{InternalToken: "secret"})
	body, _ := json.Marshal(
		map[string]string{
			"session_path": filepath.Join(service.piSessionsDir, "missing.jsonl"),
		},
	)
	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/agent-chat/import-session",
		bytes.NewReader(body),
	)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.HandleInternalPiSessionImport(c); err == nil {
		t.Fatal("HandleInternalPiSessionImport() error = nil, want unauthorized")
	}
}

func TestValidatePiSessionPathRejectsSymlinkEscape(t *testing.T) {
	service := newTestAgentChatService(t)
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	writePiSessionFile(
		t,
		outside,
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
	)
	link := filepath.Join(service.piSessionsDir, "escape.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := service.validatePiSessionPath(link); err == nil {
		t.Fatal("validatePiSessionPath() error = nil, want symlink escape rejection")
	}
}

func piImportFixtureLines(artifactPath string) []string {
	return []string{
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
		`{"type":"message","id":"user-1","timestamp":"2026-04-30T12:00:01Z","message":{"role":"user","content":"write design"}}`,
		`{"type":"message","id":"assistant-1","parentId":"user-1","timestamp":"2026-04-30T12:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"write","arguments":{"path":"` + filepath.ToSlash(
			artifactPath,
		) + `","content":"# Demo"}}]}}`,
		`{"type":"message","id":"tool-1","parentId":"assistant-1","timestamp":"2026-04-30T12:00:03Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"write","content":"Wrote file","isError":false}}`,
	}
}

func largePiImportFixtureLines(
	artifactPath string,
	entryCount, badTimestampIndex int,
) []string {
	lines := []string{
		`{"type":"session","id":"s1","timestamp":"2026-04-30T12:00:00Z","cwd":"/tmp/project"}`,
	}
	base := time.Date(2026, 4, 30, 12, 0, 1, 0, time.UTC)
	for idx := range entryCount {
		id := piEntryID(idx)
		parentField := ""
		if idx > 0 {
			parentField = `,"parentId":"` + piEntryID(idx-1) + `"`
		}
		timestamp := base.Add(time.Duration(idx) * time.Second).Format(time.RFC3339)
		if idx == badTimestampIndex {
			timestamp = "not-a-timestamp"
		}
		switch idx {
		case 0:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"user","content":"write design"}}`,
			)
		case 1:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"write","arguments":{"path":"`+filepath.ToSlash(
					artifactPath,
				)+`","content":"# Demo"}}]}}`,
			)
		case 2:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"toolResult","toolCallId":"call-1","toolName":"write","content":"Wrote file","isError":false}}`,
			)
		default:
			lines = append(
				lines,
				`{"type":"message","id":"`+id+`"`+parentField+`,"timestamp":"`+timestamp+`","message":{"role":"assistant","content":"message `+id+`"}}`,
			)
		}
	}
	return lines
}

func piEntryID(idx int) string {
	return fmt.Sprintf("entry-%03d", idx)
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
