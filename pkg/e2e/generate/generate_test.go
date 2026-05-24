package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/e2e/story"
)

func TestGenerateStoryDerivedGo(t *testing.T) {
	feature := sampleFeature()
	result, err := Generate([]story.Feature{feature}, Options{OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("generated files=%d, want 1", len(result.Files))
	}
	content := string(result.Files[0].Content)
	for _, want := range []string{
		`steps.Visit(t, ctx, "/")`,
		`steps.ExpectTextAbsent(t, ctx, "Session history")`,
		`e2e.RunScenario(t, "thoughts-workbench", "root-opens-document-workbench-with-chat"`,
		`e2e.RunScenarioWithViewport(t, "thoughts-workbench", "workbench-regions-mobile", e2e.ViewportMobile`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "playwright.") {
		t.Fatalf("generated content contains raw playwright API:\n%s", content)
	}
}

func TestCheckFresh(t *testing.T) {
	outDir := t.TempDir()
	feature := sampleFeature()
	features := []story.Feature{feature}
	opts := Options{OutputDir: outDir}
	result, err := Generate(features, opts)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := Write(result); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := CheckFresh(features, opts); err != nil {
		t.Fatalf("CheckFresh() error = %v", err)
	}
	path := filepath.Join(outDir, "thoughts_workbench_e2e_test.go")
	if err := os.WriteFile(path, []byte("package generated\n"), 0o644); err != nil {
		t.Fatalf("mutate generated file: %v", err)
	}
	if err := CheckFresh(features, opts); err == nil {
		t.Fatalf("CheckFresh() error = nil, want stale error")
	}
}

func sampleFeature() story.Feature {
	return story.Feature{
		Slug:  "thoughts-workbench",
		Title: "Thoughts workbench",
		Properties: []story.Property{{
			Slug:  "workbench-regions",
			Title: "Workbench regions",
			Dimensions: []story.Dimension{{
				Name:   "viewport",
				Values: []string{"mobile", "desktop-half", "desktop-full"},
			}},
			Then: []story.Step{{
				Verb: "expect_region_reachable",
				Args: map[string]string{"key": "thoughts.workbench.sidebar"},
			}},
		}},
		Scenarios: []story.Scenario{{
			Slug:  "root-opens-document-workbench-with-chat",
			Title: "Root opens document workbench with Chat",
			Given: []story.Step{
				{
					Verb: "authenticated_as",
					Args: map[string]string{"email": "tester@example.com"},
				},
				{
					Verb: "load_fixture",
					Args: map[string]string{"name": "thoughts-workbench.basic"},
				},
			},
			When: []story.Step{
				{Verb: "visit", Args: map[string]string{"path": "/"}},
				{
					Verb: "wait_for_feature_ready",
					Args: map[string]string{"feature": "thoughts.workbench"},
				},
			},
			Then: []story.Step{
				{
					Verb: "expect_region_visible",
					Args: map[string]string{"key": "thoughts.workbench.sidebar"},
				},
				{
					Verb: "expect_tab_selected",
					Args: map[string]string{"key": "thoughts.rightRail.chat"},
				},
				{
					Verb: "expect_text_absent",
					Args: map[string]string{"text": "Session history"},
				},
			},
		}},
	}
}
