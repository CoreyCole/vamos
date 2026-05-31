package e2ecmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/e2e/repair"
)

type FixConfig struct {
	RunDir string
	Apply  bool
}

type NeedsHumanError struct {
	Reasons []string
}

func (e NeedsHumanError) Error() string {
	return "repair plan needs human approval: " + strings.Join(e.Reasons, "; ")
}

func manifestPathForRun(runDir string) string {
	for _, name := range []string{"manifest.json", "run.json"} {
		path := filepath.Join(runDir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(runDir, "manifest.json")
}

func RunFix(ctx context.Context, cfg FixConfig) error {
	if strings.TrimSpace(cfg.RunDir) == "" {
		return fmt.Errorf("--run is required")
	}
	plan, err := repair.BuildPlan(ctx, repair.Request{
		RunManifestPath: manifestPathForRun(cfg.RunDir),
		FailuresPath:    filepath.Join(cfg.RunDir, "failures.json"),
		AllowedScopes: []repair.FixScope{
			repair.FixScopeDatastarUI,
			repair.FixScopeVamosHelpers,
			repair.FixScopeTests,
		},
	})
	if err != nil {
		return err
	}
	if err := repair.ValidatePlan(plan); err != nil {
		return err
	}
	out, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, string(out))
	if len(plan.NeedsHuman) > 0 {
		return NeedsHumanError{Reasons: plan.NeedsHuman}
	}
	if cfg.Apply {
		return repair.ApplyPlan(ctx, plan)
	}
	return nil
}
