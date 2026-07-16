package qrspicmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSessionPathMatchesExactIDInCustomSessionDir(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	want := writePiSession(t, root, "target.jsonl", "session-1", cwd)
	writePiSession(t, root, "other.jsonl", "session-2", cwd)

	got, err := ResolveSessionPath(root, "session-1", cwd)
	if err != nil {
		t.Fatalf("ResolveSessionPath error = %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolveSessionPathFindsPiCwdSubdirectory(t *testing.T) {
	root := t.TempDir()
	cwd := "/tmp/repo"
	want := writePiSession(
		t,
		filepath.Join(root, "--tmp-repo--"),
		"target.jsonl",
		"session-1",
		cwd,
	)

	got, err := ResolveSessionPath(root, "session-1", cwd)
	if err != nil {
		t.Fatalf("ResolveSessionPath error = %v", err)
	}
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolveSessionPathRejectsMissingOrDuplicate(t *testing.T) {
	root := t.TempDir()
	cwd := "/tmp/repo"
	if _, err := ResolveSessionPath(
		root,
		"missing",
		cwd,
	); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing error, got %v", err)
	}
	writePiSession(t, root, "one.jsonl", "dup", cwd)
	writePiSession(t, root, "two.jsonl", "dup", cwd)
	if _, err := ResolveSessionPath(
		root,
		"dup",
		cwd,
	); err == nil ||
		!strings.Contains(err.Error(), "multiple") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestResolveSessionPathDoesNotAcceptSiblingCWD(t *testing.T) {
	root := t.TempDir()
	cwd := "/tmp/repo"
	writePiSession(t, root, "sibling.jsonl", "target", "/tmp/other")

	_, err := ResolveSessionPath(root, "target", cwd)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found for wrong cwd, got %v", err)
	}
}

func TestExtractFinalAssistantTextFromSessionUsesLastAssistantResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(t, path, strings.Join([]string{
		sessionHeader("s", "/tmp/repo"),
		assistantLine("first qrspi_result"),
		`{"type":"message","message":{"role":"user","content":"qrspi_result user text"}}`,
		assistantLine("final qrspi_result"),
	}, "\n")+"\n")

	got, err := ExtractFinalAssistantTextFromSession(path)
	if err != nil {
		t.Fatalf("ExtractFinalAssistantTextFromSession error = %v", err)
	}
	if got != "final qrspi_result" {
		t.Fatalf("text = %q", got)
	}
}

func TestExtractFinalAssistantTextFromSessionIgnoresMalformedToolThinkingAndAborted(
	t *testing.T,
) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(t, path, strings.Join([]string{
		sessionHeader("s", "/tmp/repo"),
		`not-json`,
		`{"type":"message","message":{"role":"assistant","stopReason":"aborted","content":[{"type":"text","text":"aborted qrspi_result"}]}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"thinking","text":"qrspi_result hidden"},{"type":"tool_use","text":"qrspi_result tool"}]}}`,
		assistantLine("visible qrspi_result"),
	}, "\n")+"\n")

	got, err := ExtractFinalAssistantTextFromSession(path)
	if err != nil {
		t.Fatalf("ExtractFinalAssistantTextFromSession error = %v", err)
	}
	if got != "visible qrspi_result" {
		t.Fatalf("text = %q", got)
	}
}

func TestExtractFinalAssistantTextFromSessionErrorsWithoutQRSPIResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(
		t,
		path,
		sessionHeader("s", "/tmp/repo")+"\n"+assistantLine("plain text")+"\n",
	)

	_, err := ExtractFinalAssistantTextFromSession(path)
	if err == nil ||
		!strings.Contains(err.Error(), "no assistant text containing qrspi_result") {
		t.Fatalf("expected missing qrspi_result error, got %v", err)
	}
}

func TestLatestAssistantTerminalEvidenceDetectsProviderContextError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(t, path, strings.Join([]string{
		sessionHeader("verify-1", "/tmp/repo"),
		assistantLine("older qrspi_result"),
		providerContextErrorLine(
			"Codex error: Your input exceeds the context window of this model. Please adjust your input and try again.",
		),
	}, "\n")+"\n")

	got, ok, err := LatestAssistantTerminalEvidence(path)
	if err != nil || !ok {
		t.Fatalf("LatestAssistantTerminalEvidence = %+v %v %v", got, ok, err)
	}
	if got.SessionID != "verify-1" || got.Line != 3 ||
		got.Timestamp != "2026-07-04T23:15:59.015Z" ||
		got.StopReason != "error" ||
		!got.ContextWindowError ||
		got.EvidenceID == "" {
		t.Fatalf("evidence = %+v", got)
	}

	again, ok, err := LatestAssistantTerminalEvidence(path)
	if err != nil || !ok {
		t.Fatalf("second LatestAssistantTerminalEvidence = %+v %v %v", again, ok, err)
	}
	if again.EvidenceID != got.EvidenceID {
		t.Fatalf("evidence ID changed: %q != %q", again.EvidenceID, got.EvidenceID)
	}
}

func TestLatestAssistantTerminalEvidenceKeepsExtractorStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(
		t,
		path,
		sessionHeader(
			"s",
			"/tmp/repo",
		)+"\n"+providerContextErrorLine(
			"context window exceeded",
		)+"\n",
	)

	_, err := ExtractFinalAssistantTextFromSession(path)
	if err == nil ||
		!strings.Contains(err.Error(), "no assistant text containing qrspi_result") {
		t.Fatalf("expected no result error, got %v", err)
	}
}

func TestIsContextWindowErrorMessage(t *testing.T) {
	for _, message := range []string{
		"Codex error: Your input exceeds the context window of this model.",
		"context length exceeded",
		"context_length_exceeded",
		"maximum context tokens exceeded",
		"hit the context limit",
	} {
		if !IsContextWindowErrorMessage(message) {
			t.Fatalf("expected context-window match for %q", message)
		}
	}
	if IsContextWindowErrorMessage("network connection reset") {
		t.Fatal("unexpected context-window match")
	}
}

func TestExtractSessionEvidenceUsesActiveBranchAndStableIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(t, path, strings.Join([]string{
		sessionHeader("s", "/tmp/repo"),
		sessionEntryLineWithIDs("session_info", "info", ""),
		sessionEntryLineWithIDs("custom", "before-root", "info"),
		assistantLineWithIDs("root", "before-root", "root answer"),
		assistantLineWithIDs("abandoned", "root", "abandoned qrspi_result"),
		sessionEntryLineWithIDs("custom", "active-context", "root"),
		assistantLineWithIDs("active", "active-context", "active answer"),
	}, "\n")+"\n")

	evidence, err := ExtractSessionEvidence(path)
	if err != nil {
		t.Fatalf("ExtractSessionEvidence error = %v", err)
	}
	if len(evidence) != 2 || evidence[0].MessageID != "root" ||
		evidence[1].MessageID != "active" ||
		evidence[1].Fingerprint == "" {
		t.Fatalf("evidence = %+v", evidence)
	}
	latest, after, err := latestSessionEvidenceAfter(evidence, "root")
	if err != nil || latest.MessageID != "active" || len(after) != 1 {
		t.Fatalf("latest=%+v after=%+v err=%v", latest, after, err)
	}
}

func TestExtractSessionEvidenceKeepsLineFallbackForMinimalFixtures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(t, path, strings.Join([]string{
		sessionHeader("s", "/tmp/repo"),
		assistantLine("first"),
		assistantLine("second"),
	}, "\n")+"\n")

	evidence, err := ExtractSessionEvidence(path)
	if err != nil {
		t.Fatalf("ExtractSessionEvidence error = %v", err)
	}
	if len(evidence) != 2 || evidence[0].MessageID != "line:2" ||
		evidence[1].MessageID != "line:3" {
		t.Fatalf("evidence = %+v", evidence)
	}
}

func TestTextBlocksFromAssistantMessageAcceptsStringContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(
		t,
		path,
		sessionHeader(
			"s",
			"/tmp/repo",
		)+"\n"+`{"type":"message","message":{"role":"assistant","content":"string qrspi_result"}}`+"\n",
	)

	got, err := ExtractFinalAssistantTextFromSession(path)
	if err != nil {
		t.Fatalf("ExtractFinalAssistantTextFromSession error = %v", err)
	}
	if got != "string qrspi_result" {
		t.Fatalf("text = %q", got)
	}
}

func writePiSession(
	t *testing.T,
	sessionRoot, name, sessionID, cwd string,
	lines ...string,
) string {
	t.Helper()
	path := filepath.Join(sessionRoot, name)
	content := append([]string{sessionHeader(sessionID, cwd)}, lines...)
	writeSessionTestFile(t, path, strings.Join(content, "\n")+"\n")
	return path
}

func sessionHeader(sessionID, cwd string) string {
	return fmt.Sprintf(
		`{"type":"session","version":3,"id":%q,"timestamp":"2026-06-20T00:00:00Z","cwd":%q}`,
		sessionID,
		cwd,
	)
}

func assistantLine(text string) string {
	return fmt.Sprintf(
		`{"type":"message","message":{"role":"assistant","stopReason":"endTurn","content":[{"type":"thinking","text":"hidden"},{"type":"text","text":%q}]}}`,
		text,
	)
}

func assistantLineWithIDs(id, parentID, text string) string {
	return fmt.Sprintf(
		`{"type":"message","id":%q,"parentId":%q,"message":{"role":"assistant","stopReason":"endTurn","content":[{"type":"thinking","text":"hidden"},{"type":"text","text":%q}]}}`,
		id,
		parentID,
		text,
	)
}

func sessionEntryLineWithIDs(entryType, id, parentID string) string {
	return fmt.Sprintf(
		`{"type":%q,"id":%q,"parentId":%q}`,
		entryType,
		id,
		parentID,
	)
}

func providerContextErrorLine(message string) string {
	return fmt.Sprintf(
		`{"type":"message","timestamp":"2026-07-04T23:15:59.015Z","message":{"role":"assistant","content":[],"provider":"openai-codex","model":"gpt-5.5","stopReason":"error","errorMessage":%q}}`,
		message,
	)
}

func writeSessionTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
