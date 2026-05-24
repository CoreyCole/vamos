package runtime

import "testing"

type ViewportClass string

const (
	ViewportMobile      ViewportClass = "mobile"
	ViewportDesktopHalf ViewportClass = "desktop-half"
	ViewportDesktopFull ViewportClass = "desktop-full"
)

type Context struct {
	FeatureSlug  string
	ScenarioSlug string
	Viewport     ViewportClass
}

type ScenarioFunc func(testing.TB, *Context)

func RunScenario(t testing.TB, featureSlug, scenarioSlug string, fn ScenarioFunc) {
	t.Helper()
	fn(t, &Context{FeatureSlug: featureSlug, ScenarioSlug: scenarioSlug})
}

func RunScenarioWithViewport(t testing.TB, featureSlug, scenarioSlug string, viewport ViewportClass, fn ScenarioFunc) {
	t.Helper()
	fn(t, &Context{FeatureSlug: featureSlug, ScenarioSlug: scenarioSlug, Viewport: viewport})
}
