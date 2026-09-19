package agentchat

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/db"
	servercfg "github.com/CoreyCole/vamos/server"
	"github.com/CoreyCole/vamos/server/services/planworkspace"
)

func TestPlanRoomProcessCwdUsesWorkingCheckoutWhenImplDirEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	working := filepath.Join(root, "vamos")
	baseline := filepath.Join(root, "vamos-main")
	planRel := "thoughts/acme/plans/demo"
	planDisk := filepath.Join(root, "acme", "plans", "demo")
	if err := os.MkdirAll(planDisk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDisk, "AGENTS.md"),
		[]byte("---\nproject: github.com/coreycole/vamos\n---\n# Body\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		thoughtsRoot: root,
		defaultCwd:   filepath.Join(root, "feature"),
		projects: servercfg.ProjectsConfig{
			DefaultRepo:             "github.com/coreycole/vamos",
			DefaultCheckout:         "stage",
			DefaultBaselineCheckout: "main",
			Repos: map[string]servercfg.RepoConfig{
				"github.com/coreycole/vamos": {
					DefaultCheckout:  "stage",
					BaselineCheckout: "main",
					Checkouts: map[string]servercfg.CheckoutConfig{
						"stage": {RootPath: working},
						"main": {
							RootPath:    baseline,
							Role:        servercfg.CheckoutRoleMain,
							MustBeClean: true,
						},
					},
				},
			},
		},
	}
	_, sessionFile, cwd, inject, err := svc.prepareRoomSession(
		t.Context(),
		db.AgentThread{
			ID:         "thread-plan",
			PlanDirRel: sql.NullString{String: planRel, Valid: true},
		},
		"lead",
	)
	if err != nil {
		t.Fatal(err)
	}
	if cwd != working {
		t.Fatalf("process cwd = %q, want working %q", cwd, working)
	}
	if cwd == planDisk || cwd == baseline {
		t.Fatalf("process cwd leaked plan or baseline: %q", cwd)
	}
	if !strings.Contains(sessionFile, filepath.Join("acme", "plans", "demo")) {
		t.Fatalf("session file not on plan disk: %s", sessionFile)
	}
	content := workingDirInjectContent(inject)
	if !strings.Contains(content, "process_cwd: "+working) {
		t.Fatalf("inject missing process_cwd:\n%s", content)
	}
	if !strings.Contains(content, "plan_dir: "+planRel) {
		t.Fatalf("inject missing plan_dir:\n%s", content)
	}
	if strings.Contains(content, "impl_dir:") {
		t.Fatalf("empty impl_dir should be omitted:\n%s", content)
	}
	roomDisk, err := RoomCwdAbs(
		root,
		RoomIdentity{Kind: RoomKindPlan, PlanDirRel: planRel},
	)
	if err != nil {
		t.Fatal(err)
	}
	if roomDisk != planDisk {
		t.Fatalf("RoomCwdAbs = %q, want %q", roomDisk, planDisk)
	}
}

func TestPlanRoomProcessCwdUsesImplDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	working := filepath.Join(root, "vamos")
	copyPath := filepath.Join(root, "impl-copy")
	planRel := "thoughts/acme/plans/job"
	planDisk := filepath.Join(root, "acme", "plans", "job")
	if err := os.MkdirAll(planDisk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(planDisk, "AGENTS.md"),
		[]byte(
			"---\nproject: github.com/coreycole/vamos\nimpl_dir: "+copyPath+"\n---\n# Body\n",
		),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	svc := &Service{
		thoughtsRoot: root,
		defaultCwd:   working,
		projects: servercfg.ProjectsConfig{
			Repos: map[string]servercfg.RepoConfig{
				"github.com/coreycole/vamos": {
					DefaultCheckout: "stage",
					Checkouts: map[string]servercfg.CheckoutConfig{
						"stage": {RootPath: working},
					},
				},
			},
		},
	}
	_, _, cwd, inject, err := svc.prepareRoomSession(t.Context(), db.AgentThread{
		ID:         "thread-impl",
		PlanDirRel: sql.NullString{String: planRel, Valid: true},
	}, "lead")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != copyPath {
		t.Fatalf("process cwd = %q, want impl %q", cwd, copyPath)
	}
	content := workingDirInjectContent(inject)
	if !strings.Contains(content, "impl_dir: "+copyPath) {
		t.Fatalf("inject missing impl_dir:\n%s", content)
	}
}

func TestRecordPlanImplDirMergesFrontmatter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "AGENTS.md"),
		[]byte("---\nproject: vamos\n---\n# Body\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dir, "copy")
	if err := recordPlanImplDir(dir, copyPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	fm, err := planworkspace.ParsePlanWorkspaceFrontmatter("AGENTS.md", data)
	if err != nil {
		t.Fatal(err)
	}
	if fm.ImplDir != copyPath {
		t.Fatalf("ImplDir = %q", fm.ImplDir)
	}
	if fm.Project != "vamos" || !strings.Contains(string(data), "# Body") {
		t.Fatalf("lost other keys/body:\n%s", data)
	}
}

func TestBotHomeKeepsRoomDiskProcessCwd(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "agents", "nova")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{thoughtsRoot: root, defaultCwd: filepath.Join(root, "vamos")}
	_, _, processCwd, inject, err := svc.prepareRoomSession(t.Context(), db.AgentThread{
		ID:  "thread-home",
		Cwd: cwd,
	}, "nova")
	if err != nil {
		t.Fatal(err)
	}
	if processCwd != cwd {
		t.Fatalf("bot-home cwd = %q, want room disk %q", processCwd, cwd)
	}
	if workingDirInjectContent(inject) != "" {
		t.Fatalf("bot-home should not inject working-directory.md")
	}
}
