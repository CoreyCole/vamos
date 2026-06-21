package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestPickleballSelfModifyingStory(t *testing.T) {
	spec.Story(t, "pickleball-self-modifying").
		App(vamos.App()).
		Viewport(duiruntime.ViewportMobile).
		As(vamos.Robot).
		Do(seedPickleballStoryState()).
		Visit(vamos.Pages.Path("/examples/pickleball")).
		Expect(spec.ExpectStep(spec.Visible(spec.Text("Self-modifying pickleball")))).
		Do(spec.Click(spec.CSS("button[aria-controls='pickleball-chat-region']"))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("#pickleball-prompt-form textarea[name='prompt']")))).
		Do(spec.Fill(spec.CSS("#pickleball-prompt"), "Add a CSV column explaining skill totals.")).
		Expect(spec.InputValue(spec.CSS("#pickleball-prompt"), "Add a CSV column explaining skill totals.")).
		Do(spec.Click(spec.CSS("button[aria-controls='pickleball-state-region']"))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("#pickleball-preview iframe[src*='/thoughts/_render/html/']")))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("#pickleball-preview-link")))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("#pickleball-csv-link")))).
		Expect(spec.TextContains(spec.CSS("#pickleball-preview"), "Copy preview link")).
		Do(spec.Click(spec.CSS("#pickleball-preview summary"))).
		Expect(spec.TextContains(spec.CSS("#pickleball-preview"), "Restore source for AI")).
		Expect(spec.ExpectStep(spec.TextAbsent("build-1"))).
		Expect(vamos.Console.Clean()).
		Run()
}

func seedPickleballStoryState() spec.Step {
	return spec.Custom("seed pickleball story state", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		root := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT"))
		if root == "" {
			root = filepath.Join(ctx.Config.RepoRoot, "thoughts")
		}
		sessionID := "playwright-localhost"
		exampleRoot := filepath.Join(root, "creative-mode-agent", "examples", "pickleball")
		sessionRoot := filepath.Join(exampleRoot, "sessions", sessionID)
		workspaceRoot := filepath.Join(sessionRoot, "workspace")
		snapshotRoot := filepath.Join(sessionRoot, "snapshots", "build-1")
		for _, dir := range []string{workspaceRoot, filepath.Join(snapshotRoot, "source")} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		files := map[string]string{
			filepath.Join(workspaceRoot, "main.go"):          "package main\nfunc main(){}\n",
			filepath.Join(snapshotRoot, "source", "main.go"): "package main\nfunc main(){}\n",
			filepath.Join(snapshotRoot, "app.html"):          "<!doctype html><html><body><h1>Pickleball preview ready</h1></body></html>\n",
			filepath.Join(snapshotRoot, "results.csv"):       "court,team_a,team_b,reason\n1,Avery + Harper,Blake + Casey,Skill totals explained\n",
			filepath.Join(snapshotRoot, "manifest.json"):     `{"schema_version":1,"build_id":"build-1","mode":"one_shot","prompt_summary":"Seed build with skill totals","artifacts":{"html":"app.html","csv":"results.csv"}}` + "\n",
		}
		for path, content := range files {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
		}
		now := time.Now().UTC()
		snapshot := map[string]any{
			"build_id":           "build-1",
			"prompt_summary":     "Seed build with skill totals",
			"mode":               "one_shot",
			"status":             "succeeded",
			"snapshot_path":      "creative-mode-agent/examples/pickleball/sessions/" + sessionID + "/snapshots/build-1",
			"manifest_path":      "creative-mode-agent/examples/pickleball/sessions/" + sessionID + "/snapshots/build-1/manifest.json",
			"html_thoughts_path": "creative-mode-agent/examples/pickleball/sessions/" + sessionID + "/snapshots/build-1/app.html",
			"csv_thoughts_path":  "creative-mode-agent/examples/pickleball/sessions/" + sessionID + "/snapshots/build-1/results.csv",
			"source_hash":        "sha256:e2e-source",
			"html_hash":          "sha256:e2e-html",
			"csv_hash":           "sha256:e2e-csv",
			"created_at":         now,
		}
		writeJSONFixture(t, filepath.Join(snapshotRoot, "snapshot.json"), snapshot)
		session := map[string]any{
			"id":                 sessionID,
			"user_email":         "playwright@localhost",
			"workspace_path":     workspaceRoot,
			"current_build_id":   "build-1",
			"last_good_build_id": "build-1",
			"state":              "succeeded",
			"active_run_id":      "",
			"created_at":         now,
			"updated_at":         now,
		}
		writeJSONFixture(t, filepath.Join(sessionRoot, "current.json"), session)
	})
}

func writeJSONFixture(t testing.TB, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
