package workspaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const PlanWorkspaceBindingFilename = "plan-workspace.json"

type PlanWorkspaceBinding struct {
	PlanDir       string    `json:"plan_dir"`
	WorkspaceSlug string    `json:"workspace_slug"`
	CheckoutPath  string    `json:"checkout_path"`
	CreatedBy     string    `json:"created_by,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func PlanWorkspaceBindingPath(checkoutPath string) string {
	return filepath.Join(RuntimePaths(checkoutPath).RunDir, PlanWorkspaceBindingFilename)
}

func ReadPlanWorkspaceBinding(path string) (PlanWorkspaceBinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PlanWorkspaceBinding{}, err
	}
	var binding PlanWorkspaceBinding
	if err := json.Unmarshal(data, &binding); err != nil {
		return PlanWorkspaceBinding{}, err
	}
	return binding, nil
}

func WritePlanWorkspaceBinding(path string, binding PlanWorkspaceBinding) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if binding.UpdatedAt.IsZero() {
		binding.UpdatedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func PlanWorkspaceBindingMatches(binding PlanWorkspaceBinding, planDir string) bool {
	return normalizePlanDir(binding.PlanDir) == normalizePlanDir(planDir)
}

func normalizePlanDir(value string) string {
	value = strings.TrimSpace(filepath.ToSlash(value))
	value = strings.Trim(value, "/")
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	return value
}
