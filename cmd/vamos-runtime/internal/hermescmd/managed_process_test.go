//go:build !windows

package hermescmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMain(main *testing.M) {
	if os.Getenv("VAMOS_SUBPROCESS_HELPER") != "1" {
		_ = os.Unsetenv("HERMES_SESSION_ID")
	}
	os.Exit(main.Run())
}

func TestDeriveLaunchNonceNormativeVector(t *testing.T) {
	t.Parallel()
	const want = "ec19312204686d442e83eacc3ae23898ebae285fa94566561940517083ea7a35"
	if got := deriveLaunchNonce("pi-session-test-v1"); got != want {
		t.Fatalf("nonce = %q, want %q", got, want)
	}
}

func TestRunManagedProcessPublishesOrderedNotification(t *testing.T) {
	t.Parallel()
	plan := t.TempDir()
	piID := "pi-session-test-v1"
	hermesID := "opaque-hermes-session"
	messageID := "pi-settlement-v1-one"
	notifier := &recordingNotifier{}
	factory := func(_ context.Context, spec ProcessSpec) ManagedCommand {
		return &fakeManagedCommand{
			start: func() error {
				directory := filepath.Join(
					plan,
					".vamos",
					"sessions",
					"pi",
					piID,
					"settlements",
				)
				if err := os.MkdirAll(directory, 0o700); err != nil {
					return err
				}
				evidence := fmt.Sprintf(
					"version: 1\nhermes_session_id: %s\npi_session_id: %s\nmessage_id: %s\nraw_response: |-\n  literal response\n",
					hermesID,
					piID,
					messageID,
				)
				if err := os.WriteFile(
					filepath.Join(directory, messageID+".yaml"),
					[]byte(evidence),
					0o600,
				); err != nil {
					return err
				}

				return WriteHandoffFrame(spec.ExtraFiles[0], HandoffFrame{
					Version:     1,
					LaunchNonce: deriveLaunchNonce(piID),
					PiSessionID: piID,
					MessageID:   messageID,
				})
			},
		}
	}
	result := runManagedProcess(t.Context(), managedProcessInput{
		PlanDir: plan, PiSessionID: piID, HermesSessionID: hermesID,
		Name: "fake", Environment: os.Environ(), Stdout: io.Discard, Stderr: io.Discard,
		Notifier: notifier, ProcessFactory: factory, DrainTimeout: time.Second,
		TerminateGrace: time.Second, NotifyTimeout: time.Second, OwnerUID: os.Geteuid(),
	})
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	if len(result.Notifications) != 1 || result.Notifications[0].EventIndex != 0 {
		t.Fatalf("notifications = %+v", result.Notifications)
	}
	if len(notifier.requests) != 1 || notifier.requests[0].Message != "literal response" {
		t.Fatalf("requests = %+v", notifier.requests)
	}
}

func TestRunManagedProcessSubprocessBurstAndReap(t *testing.T) {
	t.Parallel()
	plan := t.TempDir()
	const count = 64
	notifier := &recordingNotifier{}
	result := runManagedProcess(t.Context(), managedProcessInput{
		PlanDir:         plan,
		PiSessionID:     "pi-subprocess",
		HermesSessionID: "hermes-subprocess",
		Name:            os.Args[0],
		Args:            []string{"-test.run=TestManagedProcessSubprocessHelper"},
		Environment: append(
			os.Environ(),
			"VAMOS_SUBPROCESS_HELPER=1",
			"VAMOS_SUBPROCESS_COUNT="+strconv.Itoa(count),
		),
		Stdout:         io.Discard,
		Stderr:         io.Discard,
		Notifier:       notifier,
		ProcessFactory: defaultStartWaitProcessFactory,
		DrainTimeout:   2 * time.Second,
		TerminateGrace: time.Second,
		NotifyTimeout:  time.Second,
		OwnerUID:       os.Geteuid(),
	})
	if err := result.Err(); err != nil {
		t.Fatal(err)
	}
	if len(result.Notifications) != count || len(notifier.requests) != count {
		t.Fatalf(
			"notifications=%d requests=%d",
			len(result.Notifications),
			len(notifier.requests),
		)
	}
	for index, event := range result.Notifications {
		if event.EventIndex != index {
			t.Fatalf("event %d index = %d", index, event.EventIndex)
		}
	}
}

func TestManagedProcessSubprocessHelper(t *testing.T) {
	t.Parallel()
	if os.Getenv("VAMOS_SUBPROCESS_HELPER") != "1" {
		return
	}
	count, err := strconv.Atoi(os.Getenv("VAMOS_SUBPROCESS_COUNT"))
	if err != nil {
		t.Fatal(err)
	}
	fd, err := strconv.Atoi(os.Getenv(managedHandoffFDEnvironment))
	if err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // The parent supplies a validated inherited descriptor in this subprocess fixture.
	writer := os.NewFile(uintptr(fd), "handoff")
	if writer == nil {
		t.Fatal("handoff descriptor unavailable")
	}
	defer writer.Close()
	plan, piID, hermesID := os.Getenv(
		"VAMOS_PLAN_DIR",
	), os.Getenv(
		"PI_SESSION_ID",
	), os.Getenv(
		"HERMES_SESSION_ID",
	)
	directory := filepath.Join(plan, ".vamos", "sessions", "pi", piID, "settlements")
	//nolint:gosec // The helper receives the parent-created test plan and validated Pi ID.
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := range count {
		messageID := fmt.Sprintf("message-%03d", index)
		evidence := fmt.Sprintf(
			"version: 1\nhermes_session_id: %s\npi_session_id: %s\nmessage_id: %s\nraw_response: response-%03d\n",
			hermesID,
			piID,
			messageID,
			index,
		)
		//nolint:gosec // The message ID is generated from a bounded integer in the test fixture.
		if err := os.WriteFile(
			filepath.Join(directory, messageID+".yaml"),
			[]byte(evidence),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if err := WriteHandoffFrame(
			writer,
			HandoffFrame{
				Version:     1,
				LaunchNonce: deriveLaunchNonce(piID),
				PiSessionID: piID,
				MessageID:   messageID,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunManagedProcessRejectsNonceAndReplay(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		frames []HandoffFrame
		want   string
	}{
		{name: "nonce", frames: []HandoffFrame{{Version: 1, LaunchNonce: strings.Repeat("a", 64), PiSessionID: "pi-one", MessageID: "message-one"}}, want: "nonce mismatch"},
		{name: "replay", frames: []HandoffFrame{
			{Version: 1, LaunchNonce: deriveLaunchNonce("pi-one"), PiSessionID: "pi-one", MessageID: "message-one"},
			{Version: 1, LaunchNonce: deriveLaunchNonce("pi-one"), PiSessionID: "pi-one", MessageID: "message-one"},
		}, want: "duplicate"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			plan := t.TempDir()
			directory := filepath.Join(
				plan,
				".vamos",
				"sessions",
				"pi",
				"pi-one",
				"settlements",
			)
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			evidence := "version: 1\nhermes_session_id: hermes\npi_session_id: pi-one\nmessage_id: message-one\nraw_response: x\n"
			if err := os.WriteFile(
				filepath.Join(directory, "message-one.yaml"),
				[]byte(evidence),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			factory := func(_ context.Context, spec ProcessSpec) ManagedCommand {
				return &fakeManagedCommand{start: func() error {
					for _, frame := range test.frames {
						if err := WriteHandoffFrame(
							spec.ExtraFiles[0],
							frame,
						); err != nil {
							return err
						}
					}

					return nil
				}}
			}
			result := runManagedProcess(t.Context(), managedProcessInput{
				PlanDir:         plan,
				PiSessionID:     "pi-one",
				HermesSessionID: "hermes",
				Name:            "fake",
				Environment:     os.Environ(),
				Notifier:        &recordingNotifier{},
				ProcessFactory:  factory,
				DrainTimeout:    time.Second,
				TerminateGrace:  time.Second,
				NotifyTimeout:   time.Second,
				OwnerUID:        os.Geteuid(),
			})
			if err := result.Err(); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
