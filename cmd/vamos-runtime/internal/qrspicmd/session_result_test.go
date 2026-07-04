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

func TestExtractFinalAssistantTextFromSessionDetectsProviderErrorWithoutResult(
	t *testing.T,
) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeSessionTestFile(t, path, strings.Join([]string{
		sessionHeader("s", "/tmp/repo"),
		`{"type":"message","message":{"role":"assistant","stopReason":"toolUse","content":[{"type":"text","text":"working"}]}}`,
		`{"type":"message","message":{"role":"assistant","stopReason":"error","content":[{"type":"text","text":"Codex error"}]}}`,
	}, "\n")+"\n")

	_, err := ExtractFinalAssistantTextFromSession(path)
	if !IsChildProviderError(err) {
		t.Fatalf("expected provider error, got %T %v", err, err)
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

func writeSessionTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
