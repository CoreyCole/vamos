package hermescmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

func TestValidateSafeComponent(t *testing.T) {
	for _, value := range []string{"", ".", "..", "a/b", `a\b`, "a:b", "a\x00b"} {
		if err := ValidateSafeComponent(value); err == nil {
			t.Errorf("ValidateSafeComponent(%q) succeeded", value)
		}
	}
	for _, value := range []string{"thread-1", "session_2", "ABC123"} {
		if err := ValidateSafeComponent(value); err != nil {
			t.Errorf("ValidateSafeComponent(%q): %v", value, err)
		}
	}
}

func TestStartAcceptsPlanDirectoryWithoutContextFiles(t *testing.T) {
	planDir := t.TempDir()
	var gotArgs []string
	cmd := newStartCommand(
		func(_ context.Context, _ string, args, _ []string, _, _ io.Writer) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
	)
	cmd.SetArgs([]string{"--plan", planDir, "bootstrap task"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, arg := range gotArgs {
		if strings.Contains(arg, "AGENTS.md") || strings.Contains(arg, "design.md") ||
			strings.Contains(arg, "outline.md") ||
			strings.Contains(arg, "plan.md") {
			t.Fatalf("bootstrap launch loaded missing optional context: %#v", gotArgs)
		}
	}
}

func TestRenderPiPromptKeepsManualAndManagedCompletionBoundaries(t *testing.T) {
	plan := PlanContext{PlanRel: "CoreyCole/plans/example", PlanGoal: "test"}
	manual := RenderPiPrompt(plan, "task", "", false)
	managed := RenderPiPrompt(plan, "task", "", true)
	if !strings.Contains(manual, "vamos hermes pi done") {
		t.Fatalf("manual prompt boundary = %q", manual)
	}
	for _, want := range []string{"Settlement is system-recorded.", "Do not invoke completion commands.", "no required schema, outcome, or successor."} {
		if !strings.Contains(managed, want) {
			t.Errorf("managed prompt missing %q: %q", want, managed)
		}
	}
	if strings.Contains(managed, "vamos hermes pi done") {
		t.Fatalf("managed prompt retained completion command: %q", managed)
	}
}

func TestManagedStartUsesInheritedOpaqueSessionAndParentOnlyNotifierConfig(t *testing.T) {
	ctx := testPlan(t)
	t.Setenv("HERMES_SESSION_ID", "opaque-runtime-session")
	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte(
			"gateway_url: https://gateway.example/\ningress_token: ingress-secret\nvamos_url: https://legacy.example\ncallback_token: legacy-secret\n",
		),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	var gotConfig ParentClientConfig
	var gotSpec ProcessSpec
	dependencies := managedCommandDependencies{
		processFactory: func(_ context.Context, spec ProcessSpec) ManagedCommand {
			gotSpec = spec

			return &fakeManagedCommand{}
		},
		notifierFactory: func(sessionID string, config ParentClientConfig) (sessioningress.Notifier, error) {
			if sessionID != "opaque-runtime-session" {
				t.Fatalf("session = %q", sessionID)
			}
			gotConfig = config

			return &recordingNotifier{}, nil
		},
	}
	cmd := newStartCommand(nil, dependencies)
	cmd.SetArgs([]string{"--plan", ctx.PlanDir, "--config", configPath, "task"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotConfig.GatewayBaseURL != "https://gateway.example" ||
		gotConfig.GatewayCredential != "ingress-secret" {
		t.Fatalf("config = %+v", gotConfig)
	}
	joined := strings.Join(gotSpec.Env, "\n")
	for _, forbidden := range []string{"gateway.example", "ingress-secret", "legacy.example", "legacy-secret", "VAMOS_CONFIG="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("child environment contains %q", forbidden)
		}
	}
	for _, required := range []string{"HERMES_SESSION_ID=opaque-runtime-session", "VAMOS_HERMES_HANDOFF_FD=3", "PI_SESSION_ID="} {
		if !strings.Contains(joined, required) {
			t.Fatalf("child environment missing %q: %s", required, joined)
		}
	}
}

func TestParentNotifierConfigAllowsLocalOnlyAndRejectsPartialGateway(t *testing.T) {
	t.Setenv("HERMES_HOME", t.TempDir())
	t.Setenv("VAMOS_HERMES_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	config, err := readParentClientConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if config.GatewayBaseURL != "" || config.GatewayCredential != "" {
		t.Fatalf("local config = %+v", config)
	}
	path := filepath.Join(t.TempDir(), "partial.yaml")
	if err := os.WriteFile(
		path,
		[]byte("gateway_url: https://gateway.example\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := readParentClientConfig(path); err == nil {
		t.Fatal("partial gateway config was accepted")
	}
}

func hasEnvKey(env []string, want string) bool {
	for _, entry := range env {
		key, _, found := strings.Cut(entry, "=")
		if found && key == want {
			return true
		}
	}
	return false
}

func countEnv(env []string, want string) int {
	count := 0
	for _, value := range env {
		if value == want {
			count++
		}
	}
	return count
}
