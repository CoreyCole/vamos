//go:build !windows

package hermescmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

const testIngressCredential = "ingress-secret"

type fixedNotifier struct {
	request sessioningress.EnqueueRequest
	result  sessioningress.NotifyResult
}

func (notifier *fixedNotifier) Notify(
	_ context.Context,
	request sessioningress.EnqueueRequest,
) sessioningress.NotifyResult {
	notifier.request = request

	return notifier.result
}

func createRecoveryPlan(
	t *testing.T,
	piSessionID, messageID, hermesSessionID, rawResponse string,
) string {
	t.Helper()
	planDir := t.TempDir()
	settlements := filepath.Join(
		planDir, ".vamos", "sessions", "pi", piSessionID, "settlements",
	)
	if err := os.MkdirAll(settlements, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(settlements, messageID+".yaml"),
		settlementYAML(hermesSessionID, piSessionID, messageID, rawResponse),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	return planDir
}

func TestNotifyLoadsOnlyExactSettlementAndPreservesOpaqueResponse(t *testing.T) {
	const (
		piID      = "pi-recovery"
		messageID = "message-recovery"
		hermesID  = "opaque-hermes-recovery"
		raw       = "outcome: complete\nnext: successor\ncomplete: true\n```yaml\npi done\n```\n"
	)
	createRecoveryPlan(t, piID, messageID, hermesID, "wrong plan")
	planDir := createRecoveryPlan(t, piID, messageID, hermesID, raw)
	notifier := &fixedNotifier{result: sessioningress.NotifyResult{
		Admission: true, Code: "accepted_idle", Transport: sessioningress.TransportLocal,
	}}
	factoryCalls := 0
	cmd := newNotifyCommand(
		func(sessionID string, config ParentClientConfig) (sessioningress.Notifier, error) {
			factoryCalls++
			if sessionID != hermesID {
				t.Fatalf("Hermes session = %q", sessionID)
			}
			if config.GatewayBaseURL != "" || config.GatewayCredential != "" {
				t.Fatalf("recovery was not local-only: %+v", config)
			}

			return notifier, nil
		},
	)
	t.Setenv("HERMES_HOME", t.TempDir())
	t.Setenv("VAMOS_HERMES_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", planDir, "--pi-session", piID, "--message-id", messageID,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 1 || notifier.request.Message != raw ||
		notifier.request.PiSessionID != piID || notifier.request.MessageID != messageID {
		t.Fatalf("factory calls = %d, request = %+v", factoryCalls, notifier.request)
	}
	for _, want := range []string{
		"settlement_publication: loaded_immutable_evidence",
		"notification_admission: true",
		"manager_execution: not_observed",
		"reverse_child_delivery: not_observed",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output missing %q: %s", want, output.String())
		}
	}
}

func TestNotifyRejectsMissingAndIdentityMismatchedEvidence(t *testing.T) {
	t.Parallel()
	const piID, messageID = "pi-invalid", "message-invalid"
	for _, testCase := range []struct {
		name       string
		createPlan func(*testing.T) string
	}{
		{
			name: "missing",
			createPlan: func(t *testing.T) string {
				t.Helper()
				planDir := t.TempDir()
				if err := os.MkdirAll(filepath.Join(
					planDir, ".vamos", "sessions", "pi", piID, "settlements",
				), 0o700); err != nil {
					t.Fatal(err)
				}

				return planDir
			},
		},
		{
			name: "identity mismatch",
			createPlan: func(t *testing.T) string {
				t.Helper()

				return createRecoveryPlan(
					t, piID, messageID, "opaque-invalid", "raw",
				)
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			planDir := testCase.createPlan(t)
			if testCase.name == "identity mismatch" {
				path := filepath.Join(
					planDir, ".vamos", "sessions", "pi", piID,
					"settlements", messageID+".yaml",
				)
				if err := os.WriteFile(path, settlementYAML(
					"opaque-invalid", "different-pi", messageID, "raw",
				), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			cmd := newNotifyCommand(
				func(string, ParentClientConfig) (sessioningress.Notifier, error) {
					called = true

					return &fixedNotifier{}, nil
				},
			)
			cmd.SetArgs([]string{
				"--plan", planDir, "--pi-session", piID, "--message-id", messageID,
			})
			if err := cmd.Execute(); err == nil {
				t.Fatal("invalid evidence was accepted")
			}
			if called {
				t.Fatal("invalid evidence reached notifier factory")
			}
		})
	}
}

func TestNotifyRejectsRelativePlanBeforeFilesystemOrFactory(t *testing.T) {
	t.Parallel()
	called := false
	cmd := newNotifyCommand(
		func(string, ParentClientConfig) (sessioningress.Notifier, error) {
			called = true

			return &fixedNotifier{}, nil
		},
	)
	cmd.SetArgs([]string{
		"--plan", "does-not-exist", "--pi-session", "pi-id", "--message-id", "message-id",
	})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error = %v, want absolute-plan rejection", err)
	}
	if called {
		t.Fatal("relative plan reached notifier factory")
	}
}

func TestNotifyReportsUncertaintyWithoutClaimingManagerCompletion(t *testing.T) {
	const piID, messageID = "pi-uncertain", "message-uncertain"
	planDir := createRecoveryPlan(t, piID, messageID, "opaque-uncertain", "literal")
	notifier := &fixedNotifier{result: sessioningress.NotifyResult{
		Code: "temporarily_unavailable", Detail: "gateway outcome uncertain",
		Retryable: true, Uncertain: true, Transport: sessioningress.TransportGateway,
	}}
	cmd := newNotifyCommand(
		func(string, ParentClientConfig) (sessioningress.Notifier, error) {
			return notifier, nil
		},
	)
	t.Setenv("HERMES_HOME", t.TempDir())
	t.Setenv("VAMOS_HERMES_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", planDir, "--pi-session", piID, "--message-id", messageID,
		"--format", "json",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("uncertain notification returned success")
	}
	var report NotificationReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.NotificationAdmission || !report.NotificationRetryable ||
		!report.NotificationUncertain || len(report.Events) != 1 ||
		report.Events[0].MessageID != messageID || report.Events[0].EventIndex != 0 {
		t.Fatalf("report = %+v", report)
	}
	if strings.Contains(output.String(), "manager_completion") {
		t.Fatalf("JSON claimed manager completion: %s", output.String())
	}
}

func TestNotifyReportsStaleSessionAsTerminalAdmissionFailure(t *testing.T) {
	const piID, messageID = "pi-stale", "message-stale"
	planDir := createRecoveryPlan(t, piID, messageID, "opaque-stale", "raw")
	cmd := newNotifyCommand(
		func(string, ParentClientConfig) (sessioningress.Notifier, error) {
			return &fixedNotifier{result: sessioningress.NotifyResult{
				Code: "stale_session", Detail: "session generation is stale",
				Transport: sessioningress.TransportGateway,
			}}, nil
		},
	)
	t.Setenv("HERMES_HOME", t.TempDir())
	t.Setenv("VAMOS_HERMES_CONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", planDir, "--pi-session", piID, "--message-id", messageID,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("stale session returned success")
	}
	for _, want := range []string{
		"notification_admission: false", "notification_code: stale_session",
		"notification_retryable: false", "notification_uncertain: false",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output missing %q: %s", want, output.String())
		}
	}
}

func TestNotifyConfiguredGatewayUsesOnlyParentIngressFields(t *testing.T) {
	t.Parallel()
	const piID, messageID = "pi-configured", "message-configured"
	planDir := createRecoveryPlan(t, piID, messageID, "opaque-configured", "raw")
	configPath := filepath.Join(t.TempDir(), "configured.yaml")
	if err := os.WriteFile(configPath, []byte(
		"gateway_url: https://gateway.example/\ningress_token: "+testIngressCredential+"\n"+
			"vamos_url: https://legacy.example\ncallback_token: callback-secret\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newNotifyCommand(
		func(_ string, config ParentClientConfig) (sessioningress.Notifier, error) {
			if config.GatewayBaseURL != "https://gateway.example" ||
				config.GatewayCredential != testIngressCredential {
				t.Fatalf("parent config = %+v", config)
			}

			return &fixedNotifier{result: sessioningress.NotifyResult{
				Admission: true, Code: "accepted_queued",
			}}, nil
		},
	)
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{
		"--plan", planDir, "--pi-session", piID, "--message-id", messageID,
		"--config", configPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"gateway.example", testIngressCredential, "legacy.example", "callback-secret",
	} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("output exposed %q: %s", forbidden, output.String())
		}
	}
}

func TestNotifyUsesRootInjectedFactoryAndRejectsPartialGatewayConfig(t *testing.T) {
	t.Parallel()
	const piID, messageID = "pi-root", "message-root"
	planDir := createRecoveryPlan(t, piID, messageID, "opaque-root", "raw")
	configPath := filepath.Join(t.TempDir(), "partial.yaml")
	if err := os.WriteFile(
		configPath,
		[]byte("gateway_url: https://gateway.example\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	factoryCalled := false
	cmd := newCommand(
		nil,
		nil,
		func(string, ParentClientConfig) (sessioningress.Notifier, error) {
			factoryCalled = true

			return &fixedNotifier{}, nil
		},
	)
	cmd.SetArgs([]string{
		"pi", "notify", "--plan", planDir, "--pi-session", piID,
		"--message-id", messageID, "--config", configPath,
	})
	if err := cmd.Execute(); err == nil ||
		!strings.Contains(err.Error(), "configured together") {
		t.Fatalf("error = %v, want partial gateway rejection", err)
	}
	if factoryCalled {
		t.Fatal("partial gateway configuration reached factory")
	}
}

func TestNotifyHelpNamesExactRecoveryAndAdmissionBoundary(t *testing.T) {
	t.Parallel()
	cmd := newNotifyCommand(defaultNotifierFactory)
	cmd.SetArgs([]string{"--help"})
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"--plan", "--pi-session", "--message-id",
		"Admission does not prove manager execution or reverse delivery",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help missing %q: %s", want, output.String())
		}
	}
}
