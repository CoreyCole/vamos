//go:build !darwin

package qrspicmd

import (
	"context"
	"errors"
	"time"
)

func startLaneProcess(_ []string, _ string) (LaneProcess, error) {
	return nil, errors.New("detached lanes require Darwin process-group containment")
}

func terminateLaneProcess(_ context.Context, _ *execLaneProcess, _ time.Duration) error {
	return errors.New("detached lanes require Darwin process-group containment")
}
