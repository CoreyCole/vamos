package workspaces

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestProvisionWorkspaceCreatesCopyAndMetadata(t *testing.T) {
	source := initProvisionGitRepo(t)
	planDir := filepath.Join(source, "thoughts", "agent", "plans", "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, source, "plan")
	dest := filepath.Join(t.TempDir(), "workspace")
	result, err := (&WorkspaceProvisionActivities{
		ManagerURL:   "https://manager.test",
		RestartToken: "secret",
		Now:          func() time.Time { return time.Unix(100, 0) },
	}).provision(t.Context(), WorkspaceProvisionInput{
		PlanPath:       filepath.Join(planDir, "plan.md"),
		ProjectID:      "vamos",
		WorkspaceSlug:  "feature",
		RequestedPath:  dest,
		SourceCheckout: source,
		TrunkBranch:    "main",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if result.Status != WorkspaceProvisionStatusComplete || result.WorkspacePath != dest || result.BaseCommit == "" {
		t.Fatalf("result = %+v", result)
	}
	for _, path := range []string{
		filepath.Join(dest, ".git"),
		filepath.Join(dest, ".vamos", "workspace.json"),
		filepath.Join(dest, ".vamos", "run", "workspace.env"),
		filepath.Join(dest, ".vamos", "run", "status.json"),
		filepath.Join(dest, "thoughts", "agent", "plans", "plan", "plan.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	meta, err := ReadMetadata(WorkspaceMetadataPath(dest))
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if meta.Slug != "feature" || meta.ManagerURL != "https://manager.test" || meta.RestartToken != "secret" {
		t.Fatalf("metadata = %+v", meta)
	}
	provisionMeta, ok := readProvisionMetadata(filepath.Join(dest, ".vamos", "workspace.json"))
	if !ok || provisionMeta.ProjectID != "vamos" {
		t.Fatalf("provision metadata = %+v ok=%v, want project id", provisionMeta, ok)
	}
	binding, err := ReadPlanWorkspaceBinding(PlanWorkspaceBindingPath(dest))
	if err != nil {
		t.Fatalf("ReadPlanWorkspaceBinding: %v", err)
	}
	if binding.ProjectID != "vamos" || binding.WorkspaceSlug != "feature" || binding.PlanDir != filepath.Join(planDir) {
		t.Fatalf("binding = %+v", binding)
	}
}

func TestProvisionWorkspaceBlocksDirtyDestination(t *testing.T) {
	source := initProvisionGitRepo(t)
	dest := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "untracked.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := (&WorkspaceProvisionActivities{}).provision(t.Context(), WorkspaceProvisionInput{
		PlanPath:       "thoughts/agent/plans/plan/plan.md",
		WorkspaceSlug:  "feature",
		RequestedPath:  dest,
		SourceCheckout: source,
		TrunkBranch:    "main",
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if result.Status != WorkspaceProvisionStatusBlocked {
		t.Fatalf("result = %+v", result)
	}
}

func TestProvisionWorkspaceIdempotentForMatchingMetadata(t *testing.T) {
	source := initProvisionGitRepo(t)
	dest := filepath.Join(t.TempDir(), "workspace")
	input := WorkspaceProvisionInput{PlanPath: "thoughts/agent/plans/plan/plan.md", ProjectID: "vamos", WorkspaceSlug: "feature", RequestedPath: dest, SourceCheckout: source, TrunkBranch: "main"}
	activities := &WorkspaceProvisionActivities{}
	first, err := activities.provision(t.Context(), input)
	if err != nil || first.Status != WorkspaceProvisionStatusComplete {
		t.Fatalf("first = %+v err=%v", first, err)
	}
	second, err := activities.provision(t.Context(), input)
	if err != nil {
		t.Fatalf("second err: %v", err)
	}
	if second.Status != WorkspaceProvisionStatusComplete || second.Message != "workspace already provisioned" {
		t.Fatalf("second = %+v", second)
	}
	input.ProjectID = "datastarui"
	mismatch, err := activities.provision(t.Context(), input)
	if err != nil {
		t.Fatalf("mismatch err: %v", err)
	}
	if mismatch.Status != WorkspaceProvisionStatusBlocked {
		t.Fatalf("mismatch = %+v, want blocked", mismatch)
	}
}

func initProvisionGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testRunGit(t, dir, "init", "-b", "main")
	testRunGit(t, dir, "config", "user.email", "test@example.com")
	testRunGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, dir, "init")
	return dir
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	testRunGit(t, dir, "add", ".")
	testRunGit(t, dir, "commit", "-m", msg)
}

func testRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
