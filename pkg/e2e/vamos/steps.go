package vamos

import (
	"testing"

	duiruntime "github.com/coreycole/datastarui/e2e/runtime"
	"github.com/coreycole/datastarui/e2e/spec"

	oldruntime "github.com/CoreyCole/vamos/pkg/e2e/runtime"
	oldsteps "github.com/CoreyCole/vamos/pkg/e2e/steps"
)

func legacyStep(label string, fn func(testing.TB, *oldruntime.Context)) spec.Step {
	return spec.Custom(label, func(t testing.TB, ctx *duiruntime.Context) {
		t.Helper()
		oldCtx, persist := bridgeContext(ctx)
		defer persist()
		fn(t, oldCtx)
	})
}

func WaitForFeatureReady(feature string) spec.Step {
	return legacyStep("feature ready "+feature, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.WaitForFeatureReady(t, ctx, feature)
	})
}

func FollowFirstSidebarDocumentLink() spec.Step {
	return legacyStep("follow first sidebar document link", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.FollowFirstSidebarDocumentLink(t, ctx, "first")
	})
}

func FollowFirstBreadcrumbLink() spec.Step {
	return legacyStep("follow first breadcrumb link", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.FollowFirstBreadcrumbLink(t, ctx, "first")
	})
}

func OpenFreeformChatFixture(name string) spec.Step {
	return legacyStep("open freeform chat fixture "+name, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.OpenFreeformChatFixture(t, ctx, name)
	})
}

func TranscriptContains(text string) spec.Step {
	return legacyStep("transcript contains "+text, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ExpectTranscriptContains(t, ctx, text)
	})
}

func ReloadChat() spec.Step {
	return legacyStep("reload chat", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ReloadChat(t, ctx, "current")
	})
}

func ReopenCurrentChat() spec.Step {
	return legacyStep("reopen current chat", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ReopenCurrentChat(t, ctx, "current")
	})
}

func SeedLatestWorkspaceChats(markerA, markerB string) spec.Step {
	return legacyStep("seed latest workspace chats", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.SeedLatestWorkspaceChats(t, ctx, markerA, markerB)
	})
}

func OpenWorkspaceDocumentWithoutChatParams(label string) spec.Step {
	return legacyStep("open workspace document without chat params "+label, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.OpenWorkspaceDocumentWithoutChatParams(t, ctx, label)
	})
}

func OpenPlanWorkspace(planPath string) spec.Step {
	return legacyStep("open plan workspace "+planPath, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.OpenPlanWorkspace(t, ctx, planPath)
	})
}

func OpenWorkspaceChat(label string) spec.Step {
	return legacyStep("open workspace chat "+label, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.OpenWorkspaceChat(t, ctx, label)
	})
}

func RememberFileHash(path string) spec.Step {
	return legacyStep("remember file hash "+path, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.RememberFileHash(t, ctx, path)
	})
}

func SendPiDocsReviewPrompt(marker, outputPath string) spec.Step {
	return legacyStep("send pi docs review prompt "+marker, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.SendPiDocsReviewPrompt(t, ctx, marker, outputPath)
	})
}

func WaitForChatMarker(marker string) spec.Step {
	return legacyStep("wait for chat marker "+marker, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.WaitForChatMarker(t, ctx, marker)
	})
}

func ExpectFileHashChanged(path string) spec.Step {
	return legacyStep("file hash changed "+path, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ExpectFileHashChanged(t, ctx, path)
	})
}

func ExpectPiReviewFileSections(path string) spec.Step {
	return legacyStep("pi review file sections "+path, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ExpectPiReviewFileSections(t, ctx, path)
	})
}

func ExpectOnlyFileChanged(path string) spec.Step {
	return legacyStep("only file changed "+path, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ExpectOnlyFileChanged(t, ctx, path)
	})
}

func OpenThoughtsRootChat(label string) spec.Step {
	return legacyStep("open thoughts root chat "+label, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.OpenThoughtsRootChat(t, ctx, label)
	})
}

func SendFreeformChatPrompt(marker string) spec.Step {
	return legacyStep("send freeform chat prompt "+marker, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.SendFreeformChatPrompt(t, ctx, marker)
	})
}

func WaitForLatestFreeformChatRunCompletion(label string) spec.Step {
	return legacyStep("wait for latest freeform chat run completion "+label, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.WaitForLatestFreeformChatRunCompletion(t, ctx, label)
	})
}

func SeedProjectPlanWorkspaces(projectA, projectB string) spec.Step {
	return legacyStep("seed project plan workspaces", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.SeedProjectPlanWorkspaces(t, ctx, projectA, projectB)
	})
}

func SeedLatestFreeformChatQRSPIProjectResult(project string) spec.Step {
	return legacyStep("seed latest freeform chat qrspi project result "+project, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.SeedLatestFreeformChatQRSPIProjectResult(t, ctx, project)
	})
}

func ExpectThreadMetadataProject(project string) spec.Step {
	return legacyStep("thread metadata project "+project, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ExpectThreadMetadataProject(t, ctx, project)
	})
}

func OpenThoughtsRootChatContext(label string) spec.Step {
	return legacyStep("open thoughts root chat context "+label, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.OpenThoughtsRootChatContext(t, ctx, label)
	})
}

func OpenSeededWorkspaceChat(label string) spec.Step {
	return legacyStep("open seeded workspace chat "+label, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.OpenSeededWorkspaceChat(t, ctx, label)
	})
}

func SwitchTab(key string) spec.Step {
	return legacyStep("switch tab "+key, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.SwitchTab(t, ctx, key)
	})
}

func ToggleRegion(key string) spec.Step {
	return legacyStep("toggle region "+key, func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ToggleRegion(t, ctx, key)
	})
}

func ExpectInactiveTabPanelsHidden() spec.Step {
	return legacyStep("inactive tab panels hidden", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ExpectInactiveTabPanelsHidden(t, ctx, "current")
	})
}

func ExpectConsoleClean() spec.Step {
	return legacyStep("console clean", func(t testing.TB, ctx *oldruntime.Context) {
		oldsteps.ExpectConsoleClean(t, ctx, "errors_or_warnings")
	})
}
