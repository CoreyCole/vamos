package qrspicmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLaneSpec(t *testing.T) LaneSpec {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"review", "sessions", "prompts"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prompt := filepath.Join(root, "prompts", "lane.md")
	if err := os.WriteFile(prompt, []byte("review"), 0o644); err != nil {
		t.Fatal(err)
	}
	return LaneSpec{
		ID:             "scout",
		CoordinatorID:  "child-1",
		CoordinatorGen: 1,
		Role:           LaneRoleReviewScout,
		PromptFile:     prompt,
		Cwd:            root,
		PlanDir:        root,
		ReviewDir:      filepath.Join(root, "review"),
		ReportPath:     filepath.Join(root, "review", "scout.md"),
		SessionID:      "session-1",
		SessionDir:     filepath.Join(root, "sessions"),
		Attempt:        1,
		Timeout:        time.Minute,
	}
}

func TestLaneSpec(t *testing.T) {
	spec := testLaneSpec(t)
	if err := ValidateLaneSpec(spec); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*LaneSpec){
		"unsafe ID":      func(s *LaneSpec) { s.ID = "../escape" },
		"unknown role":   func(s *LaneSpec) { s.Role = "writer" },
		"bad attempt":    func(s *LaneSpec) { s.Attempt = 3 },
		"outside report": func(s *LaneSpec) { s.ReportPath = filepath.Join(t.TempDir(), "report.md") },
		"zero timeout":   func(s *LaneSpec) { s.Timeout = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			copy := spec
			mutate(&copy)
			if err := ValidateLaneSpec(copy); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	duplicate := spec
	duplicate.ID, duplicate.SessionID = "reviewer", "session-2"
	if err := ValidateLaneSpecs(
		[]LaneSpec{spec, duplicate},
	); err == nil ||
		!strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate result: %v", err)
	}
}

func TestLaneSpecRejectsSymlinkEscape(t *testing.T) {
	spec := testLaneSpec(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(spec.ReviewDir, "escape")); err != nil {
		t.Fatal(err)
	}
	spec.ReportPath = filepath.Join(spec.ReviewDir, "escape", "report.md")
	if err := ValidateLaneSpec(spec); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
}

func TestLaneRecord(t *testing.T) {
	spec := testLaneSpec(t)
	path := LaneRecordPath(spec)
	running := LaneRecord{
		Spec:      spec,
		State:     LaneRunning,
		ErrorTail: strings.Repeat("x", maxLaneDiagnosticBytes+1),
	}
	if err := WriteLaneRecord(path, running); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLaneRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ErrorTail) != maxLaneDiagnosticBytes {
		t.Fatalf("tail length = %d", len(got.ErrorTail))
	}
	if err := WriteLaneRecord(
		path,
		LaneRecord{Spec: spec, State: LaneSuccess},
	); err != nil {
		t.Fatal(err)
	}
	if err := WriteLaneRecord(
		path,
		LaneRecord{Spec: spec, State: LaneRunning},
	); err != nil {
		t.Fatal(err)
	}
	got, err = ReadLaneRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != LaneSuccess {
		t.Fatalf("terminal record replaced with %s", got.State)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLaneRecord(path); err == nil {
		t.Fatal("expected malformed record error")
	}
}

func TestLaneReport(t *testing.T) {
	spec := testLaneSpec(t)
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected missing report error")
	}
	if err := os.Mkdir(spec.ReportPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected directory report error")
	}
	if err := os.Remove(spec.ReportPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.ReportPath, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected empty report error")
	}
	if err := os.WriteFile(
		spec.ReportPath,
		[]byte("# Findings\n\nSafe."),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err != nil {
		t.Fatal(err)
	}
	spec.ReportPath = filepath.Join(spec.ReviewDir, "not-markdown.txt")
	if err := os.WriteFile(spec.ReportPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLaneReport(spec); err == nil {
		t.Fatal("expected extension error")
	}
}
