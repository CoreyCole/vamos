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
