package hermescmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/hermes/sessioningress"
)

type recordingNotifier struct {
	requests []sessioningress.EnqueueRequest
}

func (notifier *recordingNotifier) Notify(
	_ context.Context,
	request sessioningress.EnqueueRequest,
) sessioningress.NotifyResult {
	notifier.requests = append(notifier.requests, request)

	return sessioningress.NotifyResult{
		Admission: true,
		Code:      "accepted_idle",
		Transport: sessioningress.TransportLocal,
		Attempts:  1,
	}
}

type fakeManagedCommand struct {
	start  func() error
	wait   func() error
	signal func(os.Signal) error
	kill   func() error
}

func (command *fakeManagedCommand) Start() error {
	if command.start != nil {
		return command.start()
	}

	return nil
}

func (command *fakeManagedCommand) Wait() error {
	if command.wait != nil {
		return command.wait()
	}

	return nil
}

func (command *fakeManagedCommand) Signal(signal os.Signal) error {
	if command.signal != nil {
		return command.signal(signal)
	}

	return nil
}

func (command *fakeManagedCommand) Kill() error {
	if command.kill != nil {
		return command.kill()
	}

	return nil
}

func writeManagedSettlement(spec ProcessSpec) error {
	values := make(map[string]string)
	for _, entry := range spec.Env {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	planDir := values["VAMOS_PLAN_DIR"]
	piSessionID := values["PI_SESSION_ID"]
	hermesSessionID := values["HERMES_SESSION_ID"]
	messageID := "pi-settlement-v1-command-test"
	directory := filepath.Join(
		planDir,
		".vamos",
		"sessions",
		"pi",
		piSessionID,
		"settlements",
	)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	evidence := fmt.Sprintf(
		"version: 1\nhermes_session_id: %q\npi_session_id: %q\nmessage_id: %q\nraw_response: command settled\n",
		hermesSessionID,
		piSessionID,
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
		LaunchNonce: deriveLaunchNonce(piSessionID),
		PiSessionID: piSessionID,
		MessageID:   messageID,
	})
}
