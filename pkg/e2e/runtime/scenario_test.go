package runtime

import (
	"reflect"
	"testing"
)

func TestScenarioViewportsUsesConfiguredViewportsForImplicitScenario(t *testing.T) {
	got, err := scenarioViewports(Config{Viewports: []ViewportClass{ViewportMobile, ViewportDesktopHalf}}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []ViewportClass{ViewportMobile, ViewportDesktopHalf}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scenarioViewports()=%v want %v", got, want)
	}
}

func TestScenarioViewportsExplicitViewportStaysSingle(t *testing.T) {
	got, err := scenarioViewports(Config{Viewports: DefaultVerifyViewports()}, ViewportDesktopHalf)
	if err != nil {
		t.Fatal(err)
	}
	want := []ViewportClass{ViewportDesktopHalf}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scenarioViewports()=%v want %v", got, want)
	}
}

func TestScenarioViewportsRejectsUnknownConfiguredViewport(t *testing.T) {
	_, err := scenarioViewports(Config{Viewports: []ViewportClass{"watch"}}, "")
	if err == nil {
		t.Fatal("scenarioViewports() error = nil, want unknown viewport")
	}
}
