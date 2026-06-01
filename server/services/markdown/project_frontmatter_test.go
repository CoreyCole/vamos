package markdown

import (
	"reflect"
	"testing"
)

func TestGitHubRepoKeyForQRSPIFrontmatterUsesProject(t *testing.T) {
	fm, _, err := parseFrontmatter([]byte(`---
project: github.com/CoreyCole/vamos
repository: vamos
stage: plan
plan_dir: thoughts/example/plans/demo
---
# Body
`))
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}
	if got := githubRepoKeyForFrontmatter(fm); got != "github.com/CoreyCole/vamos" {
		t.Fatalf("githubRepoKeyForFrontmatter() = %q", got)
	}
}

func TestParseFrontmatterNormalizesRelatedProjects(t *testing.T) {
	fm, _, err := parseFrontmatter([]byte(`---
project: vamos
related_projects:
  - datastarui
  - vamos
  - cn-agents
  - datastarui
---
# Body
`))
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}
	want := []string{"cn-agents", "datastarui"}
	if !reflect.DeepEqual(fm.RelatedProjects, want) {
		t.Fatalf("RelatedProjects = %#v, want %#v", fm.RelatedProjects, want)
	}
}

func TestGitHubRepoKeyForQRSPIFrontmatterIgnoresRepositoryFallback(t *testing.T) {
	fm, _, err := parseFrontmatter([]byte(`---
repository: vamos
stage: plan
plan_dir: thoughts/example/plans/demo
---
# Body
`))
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}
	if got := githubRepoKeyForFrontmatter(fm); got != "" {
		t.Fatalf("githubRepoKeyForFrontmatter() = %q, want empty", got)
	}
}

func TestGitHubRepoKeyForNonQRSPIFrontmatterKeepsRepository(t *testing.T) {
	fm, _, err := parseFrontmatter([]byte(`---
repository: vamos
topic: docs
---
# Body
`))
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}
	if got := githubRepoKeyForFrontmatter(fm); got != "vamos" {
		t.Fatalf("githubRepoKeyForFrontmatter() = %q", got)
	}
}
