package tests

import (
	"testing"

	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestWorkspacePublicSwitch(t *testing.T) {
	spec.Story(t, "workspace public switch").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.OpenManagerWorkspaces()).
		Do(vamos.SwitchToPublicWorkspaceFromEnv()).
		Expect(vamos.PublicWorkspaceAppReachableFromEnv()).
		Run()
}

func TestWorkspacePublicUnavailable(t *testing.T) {
	spec.Story(t, "workspace public unavailable").
		App(vamos.App()).
		As(vamos.Robot).
		Do(vamos.OpenPublicWorkspaceHostFromEnv()).
		Expect(vamos.PublicWorkspaceUnavailableFromEnv()).
		Run()
}
