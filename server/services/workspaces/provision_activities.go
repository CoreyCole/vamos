package workspaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type WorkspaceProvisionActivities struct {
	ManagerURL      string
	RestartToken    string
	MetadataDirName string
	Now             func() time.Time
}

func (a *WorkspaceProvisionActivities) ProvisionWorkspace(
	ctx context.Context,
	input WorkspaceProvisionInput,
) (WorkspaceProvisionResult, error) {
	return a.provision(ctx, input)
}

func (a *WorkspaceProvisionActivities) provision(
	ctx context.Context,
	input WorkspaceProvisionInput,
) (WorkspaceProvisionResult, error) {
	input = normalizeProvisionInput(input)
	if input.PlanPath == "" && input.PlanDir == "" {
		return blockedProvision(input, "missing plan path or plan directory"), nil
	}
	if input.WorkspaceSlug == "" {
		return blockedProvision(input, "missing workspace slug"), nil
	}
	if input.RequestedPath == "" {
		return blockedProvision(input, "missing requested workspace path"), nil
	}
	source := input.BaselineCheckout
	if source == "" {
		source = input.SourceCheckout
	}
	if source == "" {
		return blockedProvision(input, "missing source or baseline checkout"), nil
	}
	if _, err := os.Stat(filepath.Join(source, ".git")); err != nil {
		return blockedProvision(input, "source checkout is not a git checkout"), nil
	}
	baseCommit, err := gitOutput(ctx, source, "rev-parse", "HEAD")
	if err != nil {
		return WorkspaceProvisionResult{}, err
	}
	baseRef := input.ParentStackRef
	if baseRef == "" {
		baseRef = input.TrunkBranch
	}
	if baseRef == "" {
		baseRef = "HEAD"
	}
	metaDirName := strings.TrimSpace(a.MetadataDirName)
	if metaDirName == "" {
		metaDirName = defaultMetadataDirName
	}
	metadataPath := filepath.Join(input.RequestedPath, metaDirName, "workspace.json")
	if existing, ok := readProvisionMetadata(metadataPath); ok {
		if provisionMetadataMatches(existing, input, baseCommit) {
			return WorkspaceProvisionResult{WorkspacePath: input.RequestedPath, WorkspaceSlug: input.WorkspaceSlug, BaseRef: existing.BaseRef, BaseCommit: existing.BaseCommit, MetadataDir: filepath.Dir(metadataPath), Status: WorkspaceProvisionStatusComplete, Message: "workspace already provisioned"}, nil
		}
		return blockedProvision(input, "existing workspace metadata does not match request"), nil
	}
	if exists, dirty := existingDestinationState(ctx, input.RequestedPath); exists && dirty {
		return blockedProvision(input, "existing destination is dirty or unknown; refusing to overwrite"), nil
	}
	if err := os.RemoveAll(input.RequestedPath); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	if err := copyCheckout(ctx, source, input.RequestedPath); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	paths := RuntimePaths(input.RequestedPath, metaDirName)
	if err := EnsureRuntimeDirs(paths); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	meta := WorkspaceProvisionMetadata{Slug: input.WorkspaceSlug, PlanPath: input.PlanPath, PlanDir: input.PlanDir, WorkspacePath: input.RequestedPath, SourceCheckout: input.SourceCheckout, BaselineCheckout: input.BaselineCheckout, BaseRef: baseRef, BaseCommit: strings.TrimSpace(baseCommit), TrunkBranch: input.TrunkBranch, ParentStackRef: input.ParentStackRef, ReviewFollowup: input.ReviewFollowup, CreatedAt: now}
	if err := writeJSONFile(metadataPath, meta, 0o644); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	ws := Workspace{Slug: input.WorkspaceSlug, CheckoutPath: input.RequestedPath, MetadataDirName: metaDirName}
	store := FileBundleStore{}
	if err := store.WriteStatus(ws, RuntimeStatus{Status: StatusStopped}); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	if err := store.WriteDesired(ws, DesiredState{Desired: StatusStopped}); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	if err := store.WriteLifecycle(ws, WorkspaceLifecycleState{DesiredState: WorkspaceDesiredStopped, ObservedState: WorkspaceObservedStopped}); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	if err := store.WriteWorkspaceEnv(ws, WorkspaceEnv{Slug: input.WorkspaceSlug, CheckoutPath: input.RequestedPath, ManagerURL: a.ManagerURL, RestartToken: a.RestartToken, DatabasePath: paths.AgentsDB}); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	if err := syncProvisionPlanDir(input, input.RequestedPath); err != nil {
		return WorkspaceProvisionResult{}, err
	}
	return WorkspaceProvisionResult{WorkspacePath: input.RequestedPath, WorkspaceSlug: input.WorkspaceSlug, BaseRef: baseRef, BaseCommit: strings.TrimSpace(baseCommit), MetadataDir: filepath.Dir(metadataPath), Status: WorkspaceProvisionStatusComplete}, nil
}

func normalizeProvisionInput(input WorkspaceProvisionInput) WorkspaceProvisionInput {
	input.PlanPath = filepath.Clean(strings.TrimSpace(input.PlanPath))
	input.PlanDir = filepath.Clean(strings.TrimSpace(input.PlanDir))
	if input.PlanDir == "." || input.PlanDir == "" {
		if input.PlanPath != "." && input.PlanPath != "" {
			input.PlanDir = filepath.Dir(input.PlanPath)
		} else {
			input.PlanDir = ""
		}
	}
	input.WorkspaceSlug = strings.TrimSpace(input.WorkspaceSlug)
	input.RequestedPath = filepath.Clean(strings.TrimSpace(input.RequestedPath))
	if input.RequestedPath == "." {
		input.RequestedPath = ""
	}
	input.SourceCheckout = filepath.Clean(strings.TrimSpace(input.SourceCheckout))
	if input.SourceCheckout == "." {
		input.SourceCheckout = ""
	}
	input.BaselineCheckout = filepath.Clean(strings.TrimSpace(input.BaselineCheckout))
	if input.BaselineCheckout == "." {
		input.BaselineCheckout = ""
	}
	input.TrunkBranch = strings.TrimSpace(input.TrunkBranch)
	input.ParentStackRef = strings.TrimSpace(input.ParentStackRef)
	return input
}

func blockedProvision(input WorkspaceProvisionInput, msg string) WorkspaceProvisionResult {
	return WorkspaceProvisionResult{WorkspacePath: input.RequestedPath, WorkspaceSlug: input.WorkspaceSlug, Status: WorkspaceProvisionStatusBlocked, Message: msg}
}

func readProvisionMetadata(path string) (WorkspaceProvisionMetadata, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceProvisionMetadata{}, false
	}
	var meta WorkspaceProvisionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return WorkspaceProvisionMetadata{}, false
	}
	return meta, true
}

func provisionMetadataMatches(meta WorkspaceProvisionMetadata, input WorkspaceProvisionInput, baseCommit string) bool {
	return meta.Slug == input.WorkspaceSlug && samePath(meta.WorkspacePath, input.RequestedPath) && meta.PlanDir == input.PlanDir && strings.TrimSpace(meta.BaseCommit) == strings.TrimSpace(baseCommit)
}

func existingDestinationState(ctx context.Context, path string) (exists bool, dirty bool) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false
	}
	if err != nil {
		return true, true
	}
	if len(entries) == 0 {
		return true, false
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return true, true
	}
	out, err := gitOutput(ctx, path, "status", "--short")
	return true, err != nil || strings.TrimSpace(out) != ""
}

func copyCheckout(ctx context.Context, source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "cp", "-a", "--reflink=auto", source+string(os.PathSeparator)+".", dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("copy checkout: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func syncProvisionPlanDir(input WorkspaceProvisionInput, workspacePath string) error {
	if input.PlanDir == "" {
		return nil
	}
	if _, err := os.Stat(input.PlanDir); err != nil {
		return nil
	}
	dest := filepath.Join(workspacePath, input.PlanDir)
	if samePath(input.PlanDir, dest) {
		return nil
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("cp", "-a", input.PlanDir+string(os.PathSeparator)+".", dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sync plan dir: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
