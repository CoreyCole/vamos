package workspacecmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type RegisterOptions struct {
	ManagerURL   string
	RestartToken string
	PlanDir      string
	CreatedBy    string
}

func RunRegisterCurrent(
	ctx context.Context,
	cwd string,
	opts RegisterOptions,
	out io.Writer,
) error {
	_ = ctx
	checkout := findCheckoutRoot(cwd)
	slug, err := workspaces.SlugFromCheckoutName(filepath.Base(checkout))
	if err != nil {
		return fmt.Errorf("derive workspace slug from checkout name: %w", err)
	}
	paths := workspaces.RuntimePaths(checkout)
	oldMeta, _ := workspaces.ReadMetadata(paths.WorkspaceEnv)
	managerURL := strings.TrimSpace(opts.ManagerURL)
	if managerURL == "" {
		managerURL = oldMeta.ManagerURL
	}
	restartToken := strings.TrimSpace(opts.RestartToken)
	if restartToken == "" {
		restartToken = oldMeta.RestartToken
	}
	if managerURL == "" || restartToken == "" {
		return fmt.Errorf(
			"workspace register-current requires manager URL and restart token via flags or existing %s",
			paths.WorkspaceEnv,
		)
	}
	stale := oldMeta.Slug != "" &&
		(oldMeta.Slug != slug || !samePath(oldMeta.CheckoutPath, checkout))
	status, statusErr := readRuntimeStatus(paths.StatusJSON)
	needsStoppedState := stale || os.IsNotExist(statusErr) ||
		status.Status == "failed" || status.Status == "invalid"
	if err := workspaces.WriteMetadata(paths.WorkspaceEnv, workspaces.WorkspaceMetadata{
		Slug:         slug,
		CheckoutPath: checkout,
		ManagerURL:   managerURL,
		RestartToken: restartToken,
	}); err != nil {
		return err
	}
	if needsStoppedState {
		if err := writeStoppedRuntimeState(
			paths.StatusJSON,
			paths.DesiredJSON,
		); err != nil {
			return err
		}
		for _, path := range []string{
			paths.LifecycleJSON,
			paths.WebPID,
			paths.TemporalPID,
			paths.TSWorkerPID,
			paths.TSReadyMarker,
		} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	if strings.TrimSpace(opts.PlanDir) != "" {
		if err := workspaces.WritePlanWorkspaceBinding(
			workspaces.PlanWorkspaceBindingPath(checkout),
			workspaces.PlanWorkspaceBinding{
				PlanDir:       opts.PlanDir,
				WorkspaceSlug: slug,
				CheckoutPath:  checkout,
				CreatedBy: defaultString(
					opts.CreatedBy,
					"agentsctl workspace register-current",
				),
				UpdatedAt: time.Now().UTC(),
			},
		); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "registered workspace slug: %s\n", slug)
	fmt.Fprintf(out, "checkout: %s\n", checkout)
	if stale {
		fmt.Fprintf(
			out,
			"repaired stale runtime metadata from slug=%q checkout=%q\n",
			oldMeta.Slug,
			oldMeta.CheckoutPath,
		)
	}
	if strings.TrimSpace(opts.PlanDir) != "" {
		fmt.Fprintf(out, "plan binding: %s\n", opts.PlanDir)
	}
	return nil
}

func readRuntimeStatus(path string) (workspaceRuntimeStatusFile, error) {
	status := workspaceRuntimeStatusFile{}
	data, err := os.ReadFile(path)
	if err != nil {
		return status, err
	}
	return status, json.Unmarshal(data, &status)
}

func writeStoppedRuntimeState(statusPath, desiredPath string) error {
	if err := os.WriteFile(
		statusPath,
		[]byte("{\n  \"status\": \"stopped\",\n  \"ports\": {},\n  \"pids\": {}\n}\n"),
		0o644,
	); err != nil {
		return err
	}
	return os.WriteFile(desiredPath, []byte("{\n  \"desired\": \"stopped\"\n}\n"), 0o644)
}

func samePath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return filepath.Clean(absA) == filepath.Clean(absB)
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
