package planworkspace

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParsePlanWorkspaceFrontmatter(t *testing.T) {
	updatedAt := "2026-05-24T10:00:00Z"
	got, err := ParsePlanWorkspaceFrontmatter("AGENTS.md", []byte("---\nproject: ' example.com/alpha/app '\nqrspi_lifecycle: review_plan\nqrspi_lifecycle_updated_at: "+updatedAt+"\nqrspi_closed_reason: duplicate\n---\n# Body\n"))
	if err != nil {
		t.Fatalf("ParsePlanWorkspaceFrontmatter() error = %v", err)
	}
	if got.Project != "example.com/alpha/app" {
		t.Fatalf("Project = %q", got.Project)
	}
	if got.QRSPIStage != QRSPIStageReviewPlan {
		t.Fatalf("QRSPIStage = %q", got.QRSPIStage)
	}
	if got.QRSPIClosedReason != "duplicate" {
		t.Fatalf("QRSPIClosedReason = %q", got.QRSPIClosedReason)
	}
	if got.QRSPILifecycleUpdatedAt.IsZero() {
		t.Fatal("QRSPILifecycleUpdatedAt is zero")
	}
}

func TestParsePlanWorkspaceFrontmatterNormalizesRelatedProjects(t *testing.T) {
	got, err := ParsePlanWorkspaceFrontmatter("plan.md", []byte("---\nproject: vamos\nrelated_projects:\n  - datastarui\n  - ' vamos '\n  - ''\n  - cn-agents\n  - datastarui\n---\n# Body\n"))
	if err != nil {
		t.Fatalf("ParsePlanWorkspaceFrontmatter() error = %v", err)
	}
	want := []string{"cn-agents", "datastarui"}
	if !reflect.DeepEqual(got.RelatedProjects, want) {
		t.Fatalf("RelatedProjects = %#v, want %#v", got.RelatedProjects, want)
	}
}

func TestParsePlanWorkspaceFrontmatterDefaultsMissingFrontmatterToQuestion(t *testing.T) {
	got, err := ParsePlanWorkspaceFrontmatter("AGENTS.md", []byte("# Body\n"))
	if err != nil {
		t.Fatalf("ParsePlanWorkspaceFrontmatter() error = %v", err)
	}
	if got.QRSPIStage != QRSPIStageQuestion {
		t.Fatalf("QRSPIStage = %q, want question", got.QRSPIStage)
	}
	if got.Project != "" {
		t.Fatalf("Project = %q, want empty", got.Project)
	}
}

func TestParsePlanWorkspaceFrontmatterRejectsInvalidLifecycle(t *testing.T) {
	_, err := ParsePlanWorkspaceFrontmatter("AGENTS.md", []byte("---\nqrspi_lifecycle: bogus\n---\n# Body\n"))
	if err == nil {
		t.Fatal("ParsePlanWorkspaceFrontmatter() error = nil, want invalid lifecycle")
	}
}

func TestMergePlanWorkspaceFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte("---\nsource: test\nqrspi_lifecycle: question\n---\n# Body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	updatedAt := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	if err := MergePlanWorkspaceFrontmatter(path, PlanWorkspaceFrontmatter{
		QRSPIStage:              QRSPIStageClosed,
		QRSPILifecycleUpdatedAt: updatedAt,
		QRSPIClosedReason:       "done",
	}); err != nil {
		t.Fatalf("MergePlanWorkspaceFrontmatter() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"source: test",
		"qrspi_lifecycle: closed",
		"qrspi_lifecycle_updated_at: \"2026-05-24T10:00:00Z\"",
		"qrspi_closed_reason: done",
		"# Body",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("merged text missing %q:\n%s", want, text)
		}
	}
}

func TestIsHistoricalQRSPIStage(t *testing.T) {
	if !IsHistoricalQRSPIStage(QRSPIStageMerged) || !IsHistoricalQRSPIStage(QRSPIStageClosed) {
		t.Fatal("merged/closed should be historical")
	}
	if IsHistoricalQRSPIStage(QRSPIStageVerify) {
		t.Fatal("verify should not be historical")
	}
}
