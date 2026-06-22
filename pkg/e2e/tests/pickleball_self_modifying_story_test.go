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
		Do(seedPickleballAppletStoryState("succeeded", "Done — I updated the app and files.")).
		Visit(vamos.Pages.Path("/examples/pickleball")).
		Expect(spec.ExpectStep(spec.Visible(spec.Text("Pickleball tournament app")))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("#pickleball-preview iframe[src='/examples/pickleball/app/']")))).
		Expect(spec.TextContains(spec.CSS("#pickleball-preview"), "players.csv")).
		Expect(spec.TextContains(spec.CSS("#pickleball-preview"), "matchups.csv")).
		Expect(spec.TextContains(spec.CSS("#pickleball-preview"), "tournament.html")).
		Expect(spec.TextContains(spec.CSS("#pickleball-user-notice"), "Done — I updated the app and files.")).
		Expect(absentPickleballTechnicalTerms()).
		Expect(spec.ExpectStep(spec.TextAbsent("apps/iterations"))).
		Do(spec.Click(spec.CSS("button[aria-controls='pickleball-chat-region']"))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("#pickleball-prompt-form textarea[name='prompt']")))).
		Do(spec.Fill(spec.CSS("#pickleball-prompt"), "Add a CSV column explaining skill totals.")).
		Expect(spec.InputValue(spec.CSS("#pickleball-prompt"), "Add a CSV column explaining skill totals.")).
		Expect(spec.TextContains(spec.CSS("#pickleball-chat-panel"), "Plain-language requests are enough.")).
		Expect(absentPickleballTechnicalTerms()).
		Expect(vamos.Console.Clean()).
		Run()
}

func TestPickleballAppletFailurePreservesLastGoodStory(t *testing.T) {
	spec.Story(t, "pickleball failure preserves last-good app").
		App(vamos.App()).
		Viewport(duiruntime.ViewportMobile).
		As(vamos.Robot).
		Do(seedPickleballAppletStoryState("failed", "I couldn't make that change safely. Your app is unchanged.")).
		Visit(vamos.Pages.Path("/examples/pickleball")).
		Expect(spec.ExpectStep(spec.Visible(spec.Text("Pickleball tournament app")))).
		Expect(spec.ExpectStep(spec.Visible(spec.CSS("#pickleball-preview iframe[src='/examples/pickleball/app/']")))).
		Expect(spec.TextContains(spec.CSS("#pickleball-user-notice"), "Your app is unchanged.")).
		Expect(spec.TextContains(spec.CSS("#pickleball-chat-panel"), "App unchanged")).
		Expect(absentPickleballTechnicalTerms()).
		Expect(spec.ExpectStep(spec.TextAbsent("panic stack trace"))).
		Expect(spec.ExpectStep(spec.TextAbsent("/tmp/"))).
		Expect(vamos.Console.Clean()).
		Run()
}

func seedPickleballAppletStoryState(state, message string) spec.Step {
	return spec.Custom("seed pickleball applet story state", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		seedPickleballFilesRoot(t, ctx)
		root := strings.TrimSpace(os.Getenv("VAMOS_E2E_THOUGHTS_ROOT"))
		if root == "" {
			root = filepath.Join(ctx.Config.RepoRoot, "thoughts")
		}
		sessionID := "playwright-localhost"
		exampleRoot := filepath.Join(root, "creative-mode-agent", "examples", "pickleball")
		sessionRoot := filepath.Join(exampleRoot, "sessions", sessionID)
		workspaceRoot := filepath.Join(sessionRoot, "workspace")
		if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		session := map[string]any{
			"id":                     sessionID,
			"user_email":             "playwright@localhost",
			"workspace_path":         workspaceRoot,
			"current_iteration_id":   "last-good",
			"last_good_iteration_id": "last-good",
			"state":                  state,
			"active_run_id":          "",
			"user_message":           message,
			"error_message":          "exec failed: /tmp/app/main.go: panic stack trace",
			"log_tail":               "hidden diagnostic log",
			"created_at":             now,
			"updated_at":             now,
		}
		writeJSONFixture(t, filepath.Join(sessionRoot, "current.json"), session)
	})
}

func seedPickleballFilesRoot(t testing.TB, ctx *duiruntime.Context) {
	t.Helper()
	filesRoot := filepath.Join(ctx.Config.RepoRoot, "examples", "pickleball", "files")
	for path, content := range map[string]string{
		filepath.Join(filesRoot, "players.csv"):                                  "name,skill\nAvery,5\nBlake,4\n",
		filepath.Join(filesRoot, "matchups.csv"):                                 "court,team_a,team_b,reason\n1,Avery + Blake,Casey + Drew,Skill totals explained\n",
		filepath.Join(filesRoot, "tournament.html"):                              "<!doctype html><html><body><h1>Last good pickleball app</h1></body></html>\n",
		filepath.Join(filesRoot, "apps", "iterations", "hidden-e2e", "note.txt"): "hidden generated attempt\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func absentPickleballTechnicalTerms() spec.Expectation {
	return spec.ExpectStep(spec.Custom("pickleball normal UI hides technical terms", func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		body, err := ctx.Page.Locator("body").InnerText()
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(body)
		for _, forbidden := range []string{
			"workspace",
			"build status",
			"run id",
			"manifest",
			"promotion",
			"iteration",
			"filesystem",
			"stack trace",
			"restore source",
			"snapshot",
			"branch",
			"schema",
			"process",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("normal pickleball UI leaked %q:\n%s", forbidden, body)
			}
		}
	}))
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
