package hermescmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testPlan(t *testing.T) PlanContext {
	t.Helper()
	root := t.TempDir()
	t.Setenv("VAMOS_HERMES_RESUME_INDEX", filepath.Join(root, "manual-resume.json"))
	dir := filepath.Join(root, "thoughts", "me", "plans", "example")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "AGENTS.md"),
		[]byte(
			"---\nplan_goal: test goal\nimplementation_workspace: /tmp/impl\n---\n# Plan\n",
		),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"design.md", "outline.md", "plan.md"} {
		if err := os.WriteFile(
			filepath.Join(dir, name),
			[]byte("# "+name+"\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	ctx, err := LoadPlanContext(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestWritePiResultOverwritesCurrentConclusion(t *testing.T) {
	ctx := testPlan(t)
	_, err := WritePiResult(
		ctx,
		PiResult{
			Session: "one",
			Outcome: OutcomeHandoff,
			Next:    NextImplement,
			Summary: "1. first",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	path, err := WritePiResult(
		ctx,
		PiResult{
			Session: "one",
			Outcome: OutcomeComplete,
			Next:    NextNone,
			Summary: "1. final",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ReadPiResult(ctx.PlanDir, "one")
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeComplete || result.Summary != "1. final" {
		t.Fatalf("result=%+v", result)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
}

func TestPiCheckpointExtensionCanonicalFixture(t *testing.T) {
	checkpoint := PiCheckpoint{
		Version: 2,
		CheckpointIdentity: CheckpointIdentity{
			Session:       "session-1",
			Plan:          "CoreyCole/plans/example",
			ManagerThread: "thread-1",
			FinalEntryID:  "entry-1",
		},
		Outcome:   OutcomeComplete,
		Next:      NextNone,
		CreatedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		RawYAML:   "state: complete\nvalue: on\n",
		IntentMetadata: map[string]any{
			"alpha":   "on",
			"nested":  map[string]any{"note": "a\nb"},
			"retries": 2,
		},
	}
	got, err := CanonicalPiCheckpoint(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 2\nsession: session-1\nplan: CoreyCole/plans/example\nmanager_thread: thread-1\nfinal_entry_id: entry-1\noutcome: complete\nnext: none\ncreated_at: 2026-07-29T12:00:00Z\nraw_yaml: |\n    state: complete\n    value: on\nintent_metadata:\n    alpha: \"on\"\n    nested:\n        note: |-\n            a\n            b\n    retries: 2\n"
	if diff := string(got); diff != want {
		t.Fatalf("canonical bytes = %q, want %q", diff, want)
	}
}

func TestPiArtifactSchemaDispatchKeepsLegacyPathsDistinct(t *testing.T) {
	ctx := testPlan(t)
	tests := []struct {
		name string
		path string
		want PiArtifactSchema
	}{
		{
			name: "legacy manual result",
			path: ResultPath(ctx.PlanDir, "session-1"),
			want: PiArtifactSchema{Kind: PiArtifactLegacyResult, Version: 1},
		},
		{
			name: "legacy managed checkpoint",
			path: filepath.Join(
				ctx.PlanDir,
				".vamos",
				"sessions",
				"pi",
				"session-1",
				"checkpoints",
				"entry-1.yaml",
			),
			want: PiArtifactSchema{Kind: PiArtifactLegacyCheckpoint, Version: 2},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := PiArtifactSchemaForPath(ctx.PlanDir, test.path)
			if err != nil || got != test.want {
				t.Fatalf(
					"PiArtifactSchemaForPath() = %#v, %v; want %#v",
					got,
					err,
					test.want,
				)
			}
		})
	}
	if _, err := PiArtifactSchemaForPath(
		ctx.PlanDir,
		filepath.Join(
			ctx.PlanDir,
			".vamos",
			"sessions",
			"pi",
			"session-1",
			"checkpoints",
			"entry-1.json",
		),
	); err == nil {
		t.Fatal("accepted a checkpoint with the wrong extension")
	}
}

func TestPiCheckpointIsImmutableAndCanonical(t *testing.T) {
	ctx := testPlan(t)
	checkpoint := PiCheckpoint{
		CheckpointIdentity: CheckpointIdentity{
			Session:       "session-1",
			Plan:          ctx.PlanRel,
			ManagerThread: "thread-1",
			FinalEntryID:  "entry-1",
		},
		Outcome:   OutcomeHandoff,
		Next:      NextNone,
		CreatedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		Summary:   "settled",
		Artifacts: []string{"thoughts/me/result.md"},
	}
	path, err := WritePiCheckpoint(ctx, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalPiCheckpoint(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, canonical) {
		t.Fatalf(
			"persisted checkpoint differs from canonical bytes\nwant: %s\n got: %s",
			canonical,
			data,
		)
	}
	read, err := ReadPiCheckpoint(ctx.PlanDir, "session-1", "entry-1")
	if err != nil || read.Summary != checkpoint.Summary || read.Next != NextNone {
		t.Fatalf("ReadPiCheckpoint() = %+v, %v", read, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
}

func TestReadPiCheckpointInstalledFixture(t *testing.T) {
	plan := os.Getenv("VAMOS_PI_CHECKPOINT_FIXTURE_PLAN")
	if plan == "" {
		t.Skip("installed Pi fixture did not provide a checkpoint")
	}
	checkpoint, err := ReadPiCheckpoint(
		plan,
		os.Getenv("VAMOS_PI_CHECKPOINT_FIXTURE_SESSION"),
		os.Getenv("VAMOS_PI_CHECKPOINT_FIXTURE_ENTRY"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Next != NextNone || checkpoint.Outcome != OutcomeComplete {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
}

func TestWritePiCheckpointConcurrentIdentityBehavior(t *testing.T) {
	ctx := testPlan(t)
	checkpoint := func(summary string) PiCheckpoint {
		return PiCheckpoint{
			CheckpointIdentity: CheckpointIdentity{
				Session:       "session-1",
				Plan:          ctx.PlanRel,
				ManagerThread: "thread-1",
				FinalEntryID:  "entry-1",
			},
			Outcome:   OutcomeComplete,
			Next:      NextNone,
			Summary:   summary,
			CreatedAt: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		}
	}
	var group sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() { defer group.Done(); _, err := WritePiCheckpoint(ctx, checkpoint("same")); errs <- err }()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("same-byte writer: %v", err)
		}
	}
	_, err := WritePiCheckpoint(ctx, checkpoint("different"))
	if err == nil ||
		!strings.Contains(err.Error(), "immutable checkpoint identity conflict") {
		t.Fatalf("different-byte writer error = %v", err)
	}
	read, err := ReadPiCheckpoint(ctx.PlanDir, "session-1", "entry-1")
	if err != nil || read.Summary != "same" {
		t.Fatalf("canonical payload = %+v, %v", read, err)
	}
}

func TestPiCheckpointRejectsUnsafeIdentity(t *testing.T) {
	ctx := testPlan(t)
	_, err := WritePiCheckpoint(ctx, PiCheckpoint{
		CheckpointIdentity: CheckpointIdentity{
			Session:       "../escape",
			Plan:          ctx.PlanRel,
			ManagerThread: "thread",
			FinalEntryID:  "entry",
		},
		Outcome:   OutcomeComplete,
		Next:      NextNone,
		CreatedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("accepted traversal session")
	}
	if _, err := CheckpointPath(ctx.PlanDir, "session", "../entry"); err == nil {
		t.Fatal("accepted traversal entry")
	}
}

func TestDoneWritesResultThenNotifiesVamosFromHostConfig(t *testing.T) {
	ctx := testPlan(t)
	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte("vamos_url: https://vamos.example\ncallback_token: callback-secret\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var gotConfig hostConfig
	var gotPlan, gotSession string
	cmd := newDoneCommand(
		func(_ context.Context, config hostConfig, plan, session string) error {
			gotConfig, gotPlan, gotSession = config, plan, session
			return nil
		},
	)
	cmd.SetArgs([]string{
		"--plan", ctx.PlanDir, "--session", "session-1", "--config", configPath,
		"--outcome", "complete", "--next", "none", "--summary", "1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotConfig.CallbackToken != "callback-secret" || gotPlan != ctx.PlanDir ||
		gotSession != "session-1" {
		t.Fatalf(
			"notification = config=%+v plan=%q session=%q",
			gotConfig,
			gotPlan,
			gotSession,
		)
	}
	if _, err := ReadPiResult(ctx.PlanDir, "session-1"); err != nil {
		t.Fatalf("Pi result was not written before notification: %v", err)
	}
}

func TestNotifyPiCompletionReportsMissingHermesManager(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()
	if err := notifyPiCompletion(
		t.Context(),
		hostConfig{VamosURL: server.URL, CallbackToken: "secret"},
		"/plans/example",
		"session-1",
	); !errors.Is(err, errHermesManagerNotFound) {
		t.Fatalf("notifyPiCompletion() error = %v, want manager not found", err)
	}
}

func TestDoneUsesPiSessionID(t *testing.T) {
	ctx := testPlan(t)
	t.Setenv("PI_SESSION_ID", "pi-session")
	cmd := newDoneCommand(func(context.Context, hostConfig, string, string) error {
		t.Fatal("manual completion must not notify")
		return nil
	})
	cmd.SetArgs([]string{
		"--plan",
		ctx.PlanDir,
		"--config",
		filepath.Join(t.TempDir(), "missing-hermes.yaml"),
		"--outcome",
		"complete",
		"--next",
		"none",
		"--summary",
		"1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPiResult(ctx.PlanDir, "pi-session"); err != nil {
		t.Fatalf("Pi session result = %v", err)
	}
}

func TestStartExportsPiSessionID(t *testing.T) {
	ctx := testPlan(t)
	var gotArgs, gotEnv []string
	cmd := newStartCommand(
		func(_ context.Context, _ string, args, env []string, _, _ io.Writer) error {
			gotArgs = args
			gotEnv = env

			return nil
		},
	)
	cmd.SetArgs([]string{"--plan", ctx.PlanDir, "implement the first slice"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var session string
	for index, arg := range gotArgs {
		if arg == "--session-id" && index+1 < len(gotArgs) {
			session = gotArgs[index+1]

			break
		}
	}
	if session == "" {
		t.Fatalf("pi args missing session ID: %#v", gotArgs)
	}
	if !containsEnv(gotEnv, "PI_SESSION_ID="+session) {
		t.Fatalf("pi environment missing PI_SESSION_ID for %q", session)
	}
}

func containsEnv(env []string, want string) bool {
	for _, value := range env {
		if value == want {
			return true
		}
	}

	return false
}

func TestDoneWithoutHermesManagerPrintsManualContinuation(t *testing.T) {
	ctx := testPlan(t)
	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte("vamos_url: https://vamos.example\ncallback_token: secret\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	cmd := newDoneCommand(func(context.Context, hostConfig, string, string) error {
		return errHermesManagerNotFound
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", ctx.PlanDir, "--session", "session-1", "--config", configPath,
		"--outcome", "complete", "--next", "implement", "--summary", "1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{
		"no Hermes manager owns this session",
		"Human action required to start the recommended next session.",
		"Do not run this command from the active Pi worker.",
		"vamos hermes pi continue ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("managerless completion output = %q, want %q", got, want)
		}
	}
}

func TestDoneWithoutHostConfigRecordsLocalResult(t *testing.T) {
	ctx := testPlan(t)
	configPath := filepath.Join(t.TempDir(), "missing-hermes.yaml")
	cmd := newDoneCommand(func(context.Context, hostConfig, string, string) error {
		t.Fatal("manual completion must not notify")
		return nil
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", ctx.PlanDir, "--session", "session-1", "--config", configPath,
		"--outcome", "complete", "--next", "implement", "--summary", "1. done",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPiResult(ctx.PlanDir, "session-1"); err != nil {
		t.Fatalf("manual Pi result = %v", err)
	}
	got := output.String()
	if !strings.Contains(
		got,
		"Human action required to start the recommended next session.",
	) ||
		!strings.Contains(got, "Do not run this command from the active Pi worker.") ||
		!strings.Contains(got, "vamos hermes pi continue ") {
		t.Fatalf("manual completion output = %q", got)
	}
}

func TestEnumsRejectUnknownValues(t *testing.T) {
	if _, err := ParseOutcome("bad"); err == nil {
		t.Fatal("accepted bad outcome")
	}
	if _, err := ParseNextAction("bad"); err == nil {
		t.Fatal("accepted bad next")
	}
}

func TestPlanContextUsesThoughtsRelativeIdentity(t *testing.T) {
	ctx := testPlan(t)
	if ctx.PlanRel != "me/plans/example" || ctx.PlanGoal != "test goal" {
		t.Fatalf("context=%+v", ctx)
	}
}
