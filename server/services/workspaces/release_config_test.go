package workspaces

import (
	"os"
	"strings"
	"testing"
)

func TestBuildDefaultReleaseRegistry(t *testing.T) {
	workflows, releases, err := BuildDefaultReleaseRegistry("integration", "trunk")
	if err != nil {
		t.Fatalf("BuildDefaultReleaseRegistry() error = %v", err)
	}
	if _, ok := workflows.GetVersion(DefaultPromoteToStageWorkflowID, "v1"); !ok {
		t.Fatalf("promote workflow was not registered")
	}
	if _, ok := workflows.GetVersion(DefaultReleaseToMainWorkflowID, "v1"); !ok {
		t.Fatalf("stage-to-main workflow was not registered")
	}
	def, ok := releases.Definition(DefaultReleaseDefinitionID, "v1")
	if !ok {
		t.Fatalf("default release definition was not registered")
	}
	if got := def.Lanes[DefaultStageLaneID].CheckoutSlug; got != "integration" {
		t.Fatalf("stage checkout slug = %q, want integration", got)
	}
	if got := def.Lanes[DefaultMainLaneID].CheckoutSlug; got != "trunk" {
		t.Fatalf("main checkout slug = %q, want trunk", got)
	}
}

func TestAgentsGuideContainsWorkflowShapedGuidance(t *testing.T) {
	data, err := os.ReadFile("../../../AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(data), "Workflow-shaped feature guidance") || !strings.Contains(string(data), "pkg/agents/workflows/runtime") {
		t.Fatalf("AGENTS.md does not contain workflow-shaped guidance")
	}
}

func TestVerifyReleaseDefinitionsRequiresConfiguredLaneCheckout(t *testing.T) {
	_, releases, err := BuildDefaultReleaseRegistry("stage", "main")
	if err != nil {
		t.Fatalf("BuildDefaultReleaseRegistry() error = %v", err)
	}
	err = VerifyReleaseDefinitions(releases, map[string]ConfiguredCheckout{
		"stage": {RootPath: "/repo/stage"},
	})
	if err == nil || !strings.Contains(err.Error(), "main") {
		t.Fatalf("VerifyReleaseDefinitions() error = %v, want missing main checkout", err)
	}
	if err := VerifyReleaseDefinitions(releases, map[string]ConfiguredCheckout{
		"stage": {RootPath: "/repo/stage"},
		"main":  {RootPath: "/repo/main", IsMain: true},
	}); err != nil {
		t.Fatalf("VerifyReleaseDefinitions() error = %v", err)
	}
}
