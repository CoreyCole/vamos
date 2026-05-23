package agentchat

import "testing"

const notesRelPath = "research/notes.md"

func TestCanonicalThoughtsPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"clean", "thoughts/demo/../demo/design.md", "thoughts/demo/design.md", false},
		{"empty", "", "", true},
		{"absolute", "/tmp/thoughts/design.md", "", true},
		{"traversal", "../thoughts/design.md", "", true},
		{"missing thoughts prefix", "demo/design.md", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := CanonicalThoughtsPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("CanonicalThoughtsPath(%q) error = nil", tt.path)
				}
				return
			}
			if err != nil {
				t.Fatalf("CanonicalThoughtsPath(%q) error = %v", tt.path, err)
			}
			if got != tt.want {
				t.Fatalf("CanonicalThoughtsPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDocPathFromRoot(t *testing.T) {
	t.Parallel()
	got, err := DocPathFromRoot(
		"thoughts/creative-mode-agent/plans/demo",
		"./design.md",
	)
	if err != nil {
		t.Fatalf("DocPathFromRoot error = %v", err)
	}
	if got != "thoughts/creative-mode-agent/plans/demo/design.md" {
		t.Fatalf("DocPathFromRoot = %q", got)
	}
	got, err = DocPathFromRoot(
		"/tmp/cn-agents/thoughts/creative-mode-agent/plans/demo",
		"research/notes.md",
	)
	if err != nil {
		t.Fatalf("DocPathFromRoot(absolute) error = %v", err)
	}
	if got != "thoughts/creative-mode-agent/plans/demo/research/notes.md" {
		t.Fatalf("DocPathFromRoot(absolute) = %q", got)
	}
	if _, err := DocPathFromRoot("thoughts/demo", "../escape.md"); err == nil {
		t.Fatalf("DocPathFromRoot traversal error = nil")
	}
}

func TestRelPathFromDocPath(t *testing.T) {
	t.Parallel()
	got, err := RelPathFromDocPath(
		"thoughts/creative-mode-agent/plans/demo",
		"thoughts/creative-mode-agent/plans/demo/"+notesRelPath,
	)
	if err != nil {
		t.Fatalf("RelPathFromDocPath error = %v", err)
	}
	if got != notesRelPath {
		t.Fatalf("RelPathFromDocPath = %q", got)
	}
	got, err = RelPathFromDocPath(
		"/tmp/cn-agents/thoughts/creative-mode-agent/plans/demo",
		"thoughts/creative-mode-agent/plans/demo/design.md",
	)
	if err != nil {
		t.Fatalf("RelPathFromDocPath(absolute) error = %v", err)
	}
	if got != "design.md" {
		t.Fatalf("RelPathFromDocPath(absolute) = %q", got)
	}
	if IsDocumentUnderRoot("thoughts/demo", "thoughts/other/design.md") {
		t.Fatalf("IsDocumentUnderRoot outside root = true")
	}
}
