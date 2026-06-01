package planworkspace

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/CoreyCole/vamos/pkg/collections"
)

type QRSPIStage string

const (
	QRSPIStageQuestion             QRSPIStage = "question"
	QRSPIStageResearch             QRSPIStage = "research"
	QRSPIStageDesign               QRSPIStage = "design"
	QRSPIStageOutline              QRSPIStage = "outline"
	QRSPIStageReviewOutline        QRSPIStage = "review_outline"
	QRSPIStagePlan                 QRSPIStage = "plan"
	QRSPIStageReviewPlan           QRSPIStage = "review_plan"
	QRSPIStageWorkspace            QRSPIStage = "workspace"
	QRSPIStageImplement            QRSPIStage = "implement"
	QRSPIStageReviewImplementation QRSPIStage = "review_implementation"
	QRSPIStageVerify               QRSPIStage = "verify"
	QRSPIStageMerged               QRSPIStage = "merged"
	QRSPIStageClosed               QRSPIStage = "closed"
)

type PlanWorkspaceFrontmatter struct {
	Project                 string     `yaml:"project"`
	RelatedProjects         []string   `yaml:"related_projects"`
	QRSPIStage              QRSPIStage `yaml:"qrspi_lifecycle"`
	QRSPILifecycleUpdatedAt time.Time  `yaml:"qrspi_lifecycle_updated_at"`
	QRSPIClosedReason       string     `yaml:"qrspi_closed_reason"`
	DeclaredSource          string     `yaml:"-"`
}

func ParsePlanWorkspaceFrontmatter(path string, data []byte) (PlanWorkspaceFrontmatter, error) {
	front, _, ok := splitYAMLFrontmatter(data)
	if !ok {
		return PlanWorkspaceFrontmatter{QRSPIStage: QRSPIStageQuestion}, nil
	}
	var fm PlanWorkspaceFrontmatter
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return PlanWorkspaceFrontmatter{}, fmt.Errorf("parse %s frontmatter: %w", path, err)
	}
	fm.Project = strings.TrimSpace(fm.Project)
	fm.RelatedProjects = NormalizeRelatedProjects(fm.Project, fm.RelatedProjects)
	if fm.QRSPIStage == "" {
		fm.QRSPIStage = QRSPIStageQuestion
	}
	if !ValidQRSPIStage(fm.QRSPIStage) {
		return PlanWorkspaceFrontmatter{}, fmt.Errorf("invalid qrspi_lifecycle %q", fm.QRSPIStage)
	}
	return fm, nil
}

func NormalizeRelatedProjects(primary string, related []string) []string {
	primary = strings.TrimSpace(primary)
	seen := collections.NewSet[string]()
	out := make([]string, 0, len(related))
	for _, project := range related {
		project = strings.TrimSpace(project)
		if project == "" || project == primary || seen.Has(project) {
			continue
		}
		seen.Add(project)
		out = append(out, project)
	}
	sort.Strings(out)
	return out
}

func MergePlanWorkspaceFrontmatter(path string, update PlanWorkspaceFrontmatter) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	front, body, ok := splitYAMLFrontmatter(data)
	fields := map[string]any{}
	if ok {
		if err := yaml.Unmarshal(front, &fields); err != nil {
			return fmt.Errorf("parse %s frontmatter: %w", path, err)
		}
	} else {
		body = data
	}
	if update.QRSPIStage != "" {
		if !ValidQRSPIStage(update.QRSPIStage) {
			return fmt.Errorf("invalid qrspi_lifecycle %q", update.QRSPIStage)
		}
		fields["qrspi_lifecycle"] = string(update.QRSPIStage)
	}
	if !update.QRSPILifecycleUpdatedAt.IsZero() {
		fields["qrspi_lifecycle_updated_at"] = update.QRSPILifecycleUpdatedAt.Format(time.RFC3339)
	}
	if update.QRSPIClosedReason != "" {
		fields["qrspi_closed_reason"] = update.QRSPIClosedReason
	}
	encoded, err := yaml.Marshal(fields)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(encoded)
	out.WriteString("---\n")
	out.Write(bytes.TrimPrefix(body, []byte("\n")))
	return os.WriteFile(path, out.Bytes(), 0o644)
}

func ValidQRSPIStage(stage QRSPIStage) bool {
	switch stage {
	case QRSPIStageQuestion,
		QRSPIStageResearch,
		QRSPIStageDesign,
		QRSPIStageOutline,
		QRSPIStageReviewOutline,
		QRSPIStagePlan,
		QRSPIStageReviewPlan,
		QRSPIStageWorkspace,
		QRSPIStageImplement,
		QRSPIStageReviewImplementation,
		QRSPIStageVerify,
		QRSPIStageMerged,
		QRSPIStageClosed:
		return true
	default:
		return false
	}
}

func IsHistoricalQRSPIStage(stage QRSPIStage) bool {
	return stage == QRSPIStageMerged || stage == QRSPIStageClosed
}

func LifecycleLabel(stage QRSPIStage) string {
	switch stage {
	case QRSPIStageQuestion:
		return "Question"
	case QRSPIStageResearch:
		return "Research"
	case QRSPIStageDesign:
		return "Design"
	case QRSPIStageOutline:
		return "Outline"
	case QRSPIStageReviewOutline:
		return "Review outline"
	case QRSPIStagePlan:
		return "Plan"
	case QRSPIStageReviewPlan:
		return "Review plan"
	case QRSPIStageWorkspace:
		return "Workspace"
	case QRSPIStageImplement:
		return "Implement"
	case QRSPIStageReviewImplementation:
		return "Review implementation"
	case QRSPIStageVerify:
		return "Verify"
	case QRSPIStageMerged:
		return "Merged"
	case QRSPIStageClosed:
		return "Closed"
	default:
		return "Active"
	}
}

func splitYAMLFrontmatter(data []byte) (front []byte, body []byte, ok bool) {
	if !bytes.HasPrefix(data, []byte("---\n")) && !bytes.HasPrefix(data, []byte("---\r\n")) {
		return nil, data, false
	}
	start := 4
	if bytes.HasPrefix(data, []byte("---\r\n")) {
		start = 5
	}
	rest := data[start:]
	for offset := 0; offset < len(rest); {
		lineEnd := bytes.IndexByte(rest[offset:], '\n')
		lineLen := len(rest) - offset
		if lineEnd >= 0 {
			lineLen = lineEnd + 1
		}
		line := rest[offset : offset+lineLen]
		if strings.TrimSpace(string(line)) == "---" {
			end := start + offset
			bodyStart := end + lineLen
			return data[start:end], data[bodyStart:], true
		}
		offset += lineLen
	}
	return nil, data, false
}
