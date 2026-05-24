package story

import (
	"errors"
	"strings"
	"testing"
)

type resolverFunc func(Step) error

func (f resolverFunc) ResolveStep(step Step) error { return f(step) }

type fixtures map[string]bool

func (f fixtures) HasFixture(name string) bool { return f[name] }

func TestValidateFeatureRejectsUnknownFixture(t *testing.T) {
	feature := Feature{
		Slug:       "feature",
		Title:      "Feature",
		SourcePath: "feature.story.md",
		Scenarios: []Scenario{
			{
				Slug:  "scenario",
				Title: "Scenario",
				Given: []Step{
					{
						Verb:   "load_fixture",
						Args:   map[string]string{"name": "missing"},
						Source: SourceRange{Line: 7},
					},
				},
			},
		},
	}

	err := ValidateFeature(
		feature,
		resolverFunc(func(Step) error { return nil }),
		fixtures{},
	)
	if err == nil || !strings.Contains(err.Error(), "unknown fixture") {
		t.Fatalf("ValidateFeature() error = %v, want unknown fixture", err)
	}
}

func TestValidateFeatureRejectsUnsupportedStep(t *testing.T) {
	feature := Feature{
		Slug:       "feature",
		Title:      "Feature",
		SourcePath: "feature.story.md",
		Scenarios: []Scenario{
			{
				Slug: "scenario",
				Then: []Step{
					{
						Verb:   "unknown",
						Args:   map[string]string{},
						Source: SourceRange{Line: 9},
					},
				},
			},
		},
	}

	err := ValidateFeature(
		feature,
		resolverFunc(func(Step) error { return errors.New("unsupported step") }),
		fixtures{},
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("ValidateFeature() error = %v, want unsupported", err)
	}
}
