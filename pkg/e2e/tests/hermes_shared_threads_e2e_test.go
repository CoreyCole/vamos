package tests

import (
	"testing"

	"github.com/coreycole/datastarui/e2e/spec"

	"github.com/CoreyCole/vamos/pkg/e2e/fixtures"
	"github.com/CoreyCole/vamos/pkg/e2e/vamos"
)

func TestHermesSharedThreadBrowserIsolation(t *testing.T) {
	spec.Story(t, "Hermes shared thread browser isolation").
		App(vamos.App()).
		As(vamos.Robot).
		With(vamos.WorkspaceFixture(fixtures.HermesSharedThreadsFixture)).
		Do(vamos.OpenHermesSharedThreadFixture("primary_plan")).
		Expect(vamos.ExpectHermesSharedThreadFixture("alpha", true)).
		Expect(vamos.ExpectHermesSharedThreadURLState()).
		Do(vamos.AppendHermesFixtureLiveRefreshEvent()).
		Expect(vamos.ExpectHermesFixtureLiveRefresh()).
		Do(vamos.FollowHermesFixturePlanDocumentLink()).
		Expect(vamos.ExpectHermesSharedThreadURLState()).
		Expect(vamos.ExpectHermesSharedThreadFixture("alpha", true)).
		Do(vamos.ReloadHermesSharedThread()).
		Expect(vamos.ExpectHermesSharedThreadURLState()).
		Expect(vamos.ExpectHermesSharedThreadFixture("alpha", true)).
		Expect(vamos.ExpectHermesFixtureLiveRefresh()).
		Do(vamos.OpenHermesSharedThreadFixture("negative_plan")).
		Expect(vamos.ExpectHermesSharedThreadFixture("beta", false)).
		Do(vamos.AttemptReadOnlyHermesPrompt()).
		Expect(vamos.ExpectHermesFixtureDiskIsolation()).
		Expect(vamos.Console.Clean()).
		Run()
}
