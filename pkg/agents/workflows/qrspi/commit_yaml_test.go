package qrspi

import (
	"strings"
	"testing"
)

func validCommitFooterYAML() string {
	return "feat(qrspi): parse commit footer\n\n" +
		"```yaml\n" +
		"qrspi_commit:\n" +
		"  plan: \"thoughts/creative-mode-agent/plans/2026-06-02_17-21-45_qrspi-yaml-result-format\"\n" +
		"  stage: \"implement\"\n" +
		"  slice: \"5\"\n" +
		"  summary: \"Add fenced YAML qrspi_commit parser.\"\n" +
		"  artifacts:\n" +
		"    - \"thoughts/creative-mode-agent/plans/2026-06-02_17-21-45_qrspi-yaml-result-format/design.md\"\n" +
		"    - \"thoughts/creative-mode-agent/plans/2026-06-02_17-21-45_qrspi-yaml-result-format/outline.md\"\n" +
		"    - \"thoughts/creative-mode-agent/plans/2026-06-02_17-21-45_qrspi-yaml-result-format/plan.md\"\n" +
		"```\n"
}

func TestParseQRSPICommitFooter(t *testing.T) {
	got, err := ParseQRSPICommitFooter(validCommitFooterYAML())
	if err != nil {
		t.Fatalf("ParseQRSPICommitFooter() error = %v", err)
	}
	if got.Plan != "thoughts/creative-mode-agent/plans/2026-06-02_17-21-45_qrspi-yaml-result-format" || got.Stage != "implement" || got.Slice != "5" {
		t.Fatalf("parsed footer = %+v", got)
	}
	if got.Summary != "Add fenced YAML qrspi_commit parser." {
		t.Fatalf("summary = %q", got.Summary)
	}
	if len(got.Artifacts) != 3 || got.Artifacts[2] != "thoughts/creative-mode-agent/plans/2026-06-02_17-21-45_qrspi-yaml-result-format/plan.md" {
		t.Fatalf("artifacts = %+v", got.Artifacts)
	}
}

func TestParseQRSPICommitFooterRejectsXML(t *testing.T) {
	_, err := ParseQRSPICommitFooter(`<qrspi-commit><stage>implement</stage></qrspi-commit>`)
	if err == nil || !strings.Contains(err.Error(), "missing fenced YAML qrspi_commit") {
		t.Fatalf("ParseQRSPICommitFooter() error = %v, want missing YAML", err)
	}
}

func TestParseQRSPICommitFooterRejectsUnknownField(t *testing.T) {
	yamlText := strings.Replace(validCommitFooterYAML(), "  stage:", "  unexpected_field: true\n  stage:", 1)
	_, err := ParseQRSPICommitFooter(yamlText)
	if err == nil || !strings.Contains(err.Error(), "field unexpected_field not found") {
		t.Fatalf("ParseQRSPICommitFooter() error = %v, want unknown field", err)
	}
}

func TestParseQRSPICommitFooterWholeOutputYAMLOnlyWhenUnambiguous(t *testing.T) {
	body := `qrspi_commit:
  plan: "thoughts/example/plan"
  stage: "implement"
  slice: "5"
  summary: "Commit footer."
  artifacts:
    - "thoughts/example/plan.md"
`
	got, err := ParseQRSPICommitFooter(body)
	if err != nil {
		t.Fatalf("ParseQRSPICommitFooter() whole YAML error = %v", err)
	}
	if got.Slice != "5" {
		t.Fatalf("slice = %q", got.Slice)
	}
	_, err = ParseQRSPICommitFooter("other: true\n" + body)
	if err == nil || !strings.Contains(err.Error(), "missing fenced YAML qrspi_commit") {
		t.Fatalf("ParseQRSPICommitFooter() error = %v, want ambiguous whole-output rejection", err)
	}
}

func TestFormatQRSPICommitFooter(t *testing.T) {
	formatted := FormatQRSPICommitFooter(CommitFooter{
		Plan:      " thoughts/example ",
		Stage:     " implement ",
		Slice:     " 5 ",
		Summary:   " Add footer. ",
		Artifacts: []string{" thoughts/example/design.md ", "", " thoughts/example/plan.md "},
	})
	if !strings.HasPrefix(formatted, "```yaml\nqrspi_commit:") || !strings.HasSuffix(formatted, "\n```") {
		t.Fatalf("formatted footer = %q", formatted)
	}
	got, err := ParseQRSPICommitFooter(formatted)
	if err != nil {
		t.Fatalf("ParseQRSPICommitFooter(formatted) error = %v", err)
	}
	if got.Plan != "thoughts/example" || got.Stage != "implement" || got.Slice != "5" || got.Summary != "Add footer." {
		t.Fatalf("round trip = %+v", got)
	}
	if len(got.Artifacts) != 2 || got.Artifacts[0] != "thoughts/example/design.md" || got.Artifacts[1] != "thoughts/example/plan.md" {
		t.Fatalf("round-trip artifacts = %+v", got.Artifacts)
	}
}
