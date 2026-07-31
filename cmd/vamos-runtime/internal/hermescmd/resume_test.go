package hermescmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContinueLaunchesMappedPreviousSession(t *testing.T) {
	ctx := testPlan(t)
	artifact := filepath.Join(ctx.PlanDir, "reviews", "previous.md")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("# Previous\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePiResult(
		ctx,
		PiResult{
			Session:  "previous",
			Outcome:  OutcomeHandoff,
			Next:     NextVerify,
			Artifact: "thoughts/me/plans/example/reviews/previous.md",
			Summary:  "1. done",
		},
	); err != nil {
		t.Fatal(err)
	}
	id, err := recordManualResume(ctx, PiResult{Session: "previous"})
	if err != nil {
		t.Fatal(err)
	}
	var gotArgs, gotEnv []string
	cmd := newContinueCommand(
		func(_ context.Context, _ string, args, env []string, _, _ io.Writer) error {
			gotArgs = args
			gotEnv = env

			return nil
		},
	)
	cmd.SetArgs([]string{id})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		strings.Join(gotArgs, " "),
		"--session-dir "+ctx.PlanDir+"/.vamos/sessions/pi",
	) || len(gotArgs) < 13 {
		t.Fatalf("continue args = %#v", gotArgs)
	}
	root, err := piResourceRoot()
	if err != nil {
		t.Fatal(err)
	}
	wantContext := []string{
		"@" + filepath.Join(ctx.PlanDir, "AGENTS.md"),
		"@" + filepath.Join(ctx.PlanDir, "design.md"),
		"@" + filepath.Join(ctx.PlanDir, "outline.md"),
		"@" + filepath.Join(ctx.PlanDir, "plan.md"),
		"@" + filepath.Join(root, ".pi", "skills", "qrspi-planning", "SKILL.md"),
		"@" + filepath.Join(root, ".pi", "skills", "q-resume", "SKILL.md"),
		"@" + filepath.Join(root, ".pi", "skills", "q-handoff", "SKILL.md"),
		"@" + filepath.Join(root, ".pi", "skills", "q-verify", "SKILL.md"),
		"@" + artifact,
	}
	for i, want := range wantContext {
		if gotArgs[4+i] != want {
			t.Fatalf("context arg %d = %q, want %q", i, gotArgs[4+i], want)
		}
	}
	if !containsEnv(gotEnv, "PI_SESSION_ID="+gotArgs[1]) {
		t.Fatalf(
			"continue environment missing PI_SESSION_ID for %q",
			gotArgs[1],
		)
	}
}

func TestContinueRegistersManagedPiRunBeforeLaunchingWorker(t *testing.T) {
	ctx := testPlan(t)
	if _, err := WritePiResult(
		ctx,
		PiResult{
			Session: "previous",
			Outcome: OutcomeHandoff,
			Next:    NextImplement,
			Summary: "1. done",
		},
	); err != nil {
		t.Fatal(err)
	}
	id, err := recordManualResume(ctx, PiResult{Session: "previous"})
	if err != nil {
		t.Fatal(err)
	}
	var gotPlan, gotSession string
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)

				return
			}
			if r.URL.Path != "/agent-chat/api/hermes/threads/thread-1/events" {
				t.Fatalf("request path = %q", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer callback-secret" {
				t.Fatal("missing callback authorization")
			}
			var event struct {
				PlanDir     string `json:"plan_dir"`
				Type        string `json:"type"`
				PiSessionID string `json:"pi_session_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				t.Fatal(err)
			}
			if event.Type != "pi_run" {
				t.Fatalf("event type = %q", event.Type)
			}
			gotPlan, gotSession = event.PlanDir, event.PiSessionID
			w.WriteHeader(http.StatusAccepted)
		}),
	)
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte(
			"gateway_url: "+server.URL+"\nvamos_url: "+server.URL+"\ningress_token: ingress-secret\ncallback_token: callback-secret\n",
		),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var workerSession string
	cmd := newContinueCommand(
		func(_ context.Context, _ string, args, _ []string, _, _ io.Writer) error {
			for index, arg := range args {
				if arg == "--session-id" && index+1 < len(args) {
					workerSession = args[index+1]

					break
				}
			}

			return nil
		},
	)
	cmd.SetArgs([]string{"--thread-id", "thread-1", "--config", configPath, id})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotPlan != ctx.PlanDir || gotSession == "" || gotSession != workerSession {
		t.Fatalf(
			"registration plan=%q session=%q worker=%q",
			gotPlan,
			gotSession,
			workerSession,
		)
	}
}
