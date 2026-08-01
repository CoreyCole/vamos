package hermescmd

import (
	"context"
	"os"

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
	start func() error
	wait  func() error
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
func (*fakeManagedCommand) Signal(os.Signal) error { return nil }
func (*fakeManagedCommand) Kill() error            { return nil }
