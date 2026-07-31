package hermescmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestManagedPiEnvironmentReplacesInheritedValues(t *testing.T) {
	env := managedPiEnvironment([]string{
		"KEEP=value",
		"VAMOS_PLAN_DIR=stale-one",
		"VAMOS_THOUGHTS_ROOT=stale-one",
		"PI_SESSION_ID=stale",
		"VAMOS_PLAN_DIR=stale-two",
		"VAMOS_THOUGHTS_ROOT=stale-two",
		"VAMOS_HERMES_THREAD_ID=stale",
		managerWakeManagerThreadID + "=stale-one",
		managerWakeManagerThreadID + "=stale-two",
		managerWakePiSessionID + "=stale",
		managerWakeGatewayURL + "=https://stale.example",
		managerWakeIngressToken + "=stale-one",
		managerWakeIngressToken + "=stale-two",
	}, "/tmp/thoughts/CoreyCole/plans/example", "session-1", "thread-1", hostConfig{
		GatewayURL:    " https://gateway.example ",
		IngressToken:  "ingress-secret",
		CallbackToken: "callback-secret",
	})
	for _, want := range []string{
		"VAMOS_PLAN_DIR=/tmp/thoughts/CoreyCole/plans/example",
		"VAMOS_THOUGHTS_ROOT=/tmp/thoughts",
		"PI_SESSION_ID=session-1",
		managerWakeManagerThreadID + "=thread-1",
		managerWakePiSessionID + "=session-1",
		managerWakeGatewayURL + "=https://gateway.example",
		managerWakeIngressToken + "=ingress-secret",
	} {
		if countEnv(env, want) != 1 {
			t.Errorf(
				"environment has %d copies of %q: %#v",
				countEnv(env, want),
				want,
				env,
			)
		}
	}
	if !containsEnv(env, "KEEP=value") {
		t.Fatalf("environment dropped unrelated value: %#v", env)
	}
}

func TestManagedPiEnvironmentOmitsManagerWakeValuesWithoutManagedOwner(t *testing.T) {
	env := managedPiEnvironment([]string{
		"VAMOS_HERMES_THREAD_ID=stale",
		managerWakeManagerThreadID + "=stale",
		managerWakePiSessionID + "=stale",
		managerWakeGatewayURL + "=https://stale.example",
		managerWakeIngressToken + "=stale",
		"KEEP=value",
	}, "/tmp/thoughts/CoreyCole/plans/example", "session-1", "", hostConfig{})
	for _, name := range []string{
		"VAMOS_HERMES_THREAD_ID",
		managerWakeManagerThreadID,
		managerWakePiSessionID,
		managerWakeGatewayURL,
		managerWakeIngressToken,
	} {
		if hasEnvKey(env, name) {
			t.Fatalf("environment retained %q: %#v", name, env)
		}
	}
	for _, want := range []string{
		"VAMOS_PLAN_DIR=/tmp/thoughts/CoreyCole/plans/example",
		"VAMOS_THOUGHTS_ROOT=/tmp/thoughts",
		"PI_SESSION_ID=session-1",
		"KEEP=value",
	} {
		if countEnv(env, want) != 1 {
			t.Errorf(
				"environment has %d copies of %q: %#v",
				countEnv(env, want),
				want,
				env,
			)
		}
	}
}

func TestStartRejectsUnsafeManagedThreadWithoutLaunching(t *testing.T) {
	ctx := testPlan(t)
	for _, threadID := range []string{".", "..", "thread/escape", `thread\escape`} {
		t.Run(threadID, func(t *testing.T) {
			launched := false
			cmd := newStartCommand(
				func(context.Context, string, []string, []string, io.Writer, io.Writer) error {
					launched = true
					return nil
				},
			)
			cmd.SetArgs([]string{"--plan", ctx.PlanDir, "--thread-id", threadID, "task"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "validate Hermes thread ID") {
				t.Fatalf("error = %v, want invalid thread error", err)
			}
			if launched {
				t.Fatal("runner launched after unsafe thread ID")
			}
		})
	}
}

func TestStartDoesNotLaunchWhenRegistrationFails(t *testing.T) {
	ctx := testPlan(t)
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)

				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "hermes.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte(
			"gateway_url: "+server.URL+"\nvamos_url: "+server.URL+"\ningress_token: ingress-token\ncallback_token: token\n",
		),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	launched := false
	cmd := newStartCommand(
		func(context.Context, string, []string, []string, io.Writer, io.Writer) error {
			launched = true
			return nil
		},
	)
	cmd.SetArgs(
		[]string{
			"--plan",
			ctx.PlanDir,
			"--thread-id",
			"thread-1",
			"--config",
			configPath,
			"task",
		},
	)
	if err := cmd.Execute(); err == nil ||
		!strings.Contains(err.Error(), "register managed Pi run") {
		t.Fatalf("error = %v, want registration failure", err)
	}
	if launched {
		t.Fatal("runner launched after failed registration")
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
			strings.Contains(arg, "outline.md") || strings.Contains(arg, "plan.md") {
			t.Fatalf("bootstrap launch loaded missing optional context: %#v", gotArgs)
		}
	}
}

func TestRenderPiPromptKeepsManualAndManagedCompletionBoundaries(t *testing.T) {
	plan := PlanContext{PlanRel: "CoreyCole/plans/example", PlanGoal: "test"}
	manual := RenderPiPrompt(plan, "task", "", false)
	managed := RenderPiPrompt(plan, "task", "", true)
	if !strings.Contains(manual, "vamos hermes pi done") ||
		strings.Contains(manual, "do not invoke completion commands") {
		t.Fatalf("manual prompt boundary = %q", manual)
	}
	for _, want := range []string{
		"Settlement is system-recorded.",
		"Do not invoke completion commands.",
		"fenced YAML or YML block is optional opaque evidence only",
		"no required schema, outcome, or successor.",
	} {
		if !strings.Contains(managed, want) {
			t.Errorf("managed prompt missing %q: %q", want, managed)
		}
	}
	for _, unwanted := range []string{
		"vamos hermes pi done",
		"lifecycle intent",
	} {
		if strings.Contains(managed, unwanted) {
			t.Errorf("managed prompt retained %q: %q", unwanted, managed)
		}
	}
}

func TestManagedPiEnvironmentUsesIngressTokenInsteadOfCallbackToken(t *testing.T) {
	for _, test := range []struct {
		name          string
		ingressToken  string
		callbackToken string
	}{
		{name: "different credentials", ingressToken: "ingress", callbackToken: "callback"},
		{name: "callback resembles ingress", ingressToken: "ingress-2", callbackToken: "ingress"},
	} {
		t.Run(test.name, func(t *testing.T) {
			env := managedPiEnvironment(
				nil,
				"/tmp/thoughts/plan",
				"session",
				"thread",
				hostConfig{
					GatewayURL:    "https://gateway.example",
					IngressToken:  test.ingressToken,
					CallbackToken: test.callbackToken,
				},
			)
			if !containsEnv(env, managerWakeIngressToken+"="+test.ingressToken) {
				t.Fatalf("manager wake ingress token was not injected")
			}
			if containsEnv(env, managerWakeIngressToken+"="+test.callbackToken) &&
				test.ingressToken != test.callbackToken {
				t.Fatal("callback token populated manager wake ingress token")
			}
		})
	}
}

func TestStartRefusesIncompleteManagedConfigBeforeRunner(t *testing.T) {
	ctx := testPlan(t)
	for _, test := range []struct {
		name   string
		config string
	}{
		{name: "missing ingress token", config: "gateway_url: https://gateway.example\nvamos_url: https://vamos.example\ncallback_token: callback\n"},
		{name: "missing callback token", config: "gateway_url: https://gateway.example\nvamos_url: https://vamos.example\ningress_token: ingress\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "hermes.yaml")
			if err := os.WriteFile(configPath, []byte(test.config), 0o600); err != nil {
				t.Fatal(err)
			}
			launched := false
			cmd := newStartCommand(
				func(context.Context, string, []string, []string, io.Writer, io.Writer) error {
					launched = true
					return nil
				},
			)
			cmd.SetArgs(
				[]string{
					"--plan",
					ctx.PlanDir,
					"--thread-id",
					"thread-1",
					"--config",
					configPath,
					"task",
				},
			)
			if err := cmd.Execute(); err == nil ||
				!strings.Contains(err.Error(), "credential") {
				t.Fatalf("error = %v, want incomplete managed configuration", err)
			}
			if launched {
				t.Fatal("runner launched with incomplete managed configuration")
			}
		})
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
