package story

import "fmt"

func ValidateFeature(
	feature Feature,
	steps StepResolver,
	fixtures FixtureResolver,
) error {
	if feature.Slug == "" || feature.Title == "" {
		return fmt.Errorf("feature missing slug/title")
	}
	if len(feature.Scenarios) == 0 {
		return fmt.Errorf("feature %s has no scenarios", feature.Slug)
	}
	scenarios := append([]Scenario{}, feature.Scenarios...)
	properties, err := ExpandProperties(feature)
	if err != nil {
		return err
	}
	scenarios = append(scenarios, properties...)
	for _, scenario := range scenarios {
		for _, step := range scenarioSteps(scenario) {
			if err := steps.ResolveStep(step); err != nil {
				return fmt.Errorf("%s:%d: %w", feature.SourcePath, step.Source.Line, err)
			}
			if step.Verb == "load_fixture" && !fixtures.HasFixture(step.Args["name"]) {
				return fmt.Errorf(
					"%s:%d: unknown fixture %q",
					feature.SourcePath,
					step.Source.Line,
					step.Args["name"],
				)
			}
		}
	}
	return nil
}

func scenarioSteps(s Scenario) []Step {
	steps := make([]Step, 0, len(s.Given)+len(s.When)+len(s.Then))
	steps = append(steps, s.Given...)
	steps = append(steps, s.When...)
	steps = append(steps, s.Then...)
	return steps
}
