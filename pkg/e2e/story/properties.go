package story

import (
	"fmt"
	"strings"
)

func ExpandProperties(feature Feature) ([]Scenario, error) {
	out := []Scenario{}
	for _, prop := range feature.Properties {
		if err := ValidatePropertyDimensions(prop); err != nil {
			return nil, err
		}
		for _, combo := range expandDimensions(prop.Dimensions) {
			slug := prop.Slug
			title := prop.Title
			given := inheritedPropertyGiven(feature)
			when := []Step{}
			if viewport := combo["viewport"]; viewport != "" {
				slug += "-" + slugify(viewport)
				title += " / " + viewport
			}
			if route := combo["route"]; route != "" {
				if routeSlug := slugify(route); routeSlug != "" {
					slug += "-" + routeSlug
				}
				title += " / " + route
				when = append(
					when,
					Step{
						Kind: "when",
						Verb: "visit",
						Args: map[string]string{"path": route},
					},
					Step{
						Kind: "when",
						Verb: "wait_for_feature_ready",
						Args: map[string]string{"feature": featureReadyKey(feature.Slug)},
					},
				)
			}
			out = append(out, Scenario{
				Slug:     slug,
				Title:    title,
				Viewport: combo["viewport"],
				Given:    given,
				When:     when,
				Then:     append([]Step{}, prop.Then...),
			})
		}
	}
	return out, nil
}

func inheritedPropertyGiven(feature Feature) []Step {
	if len(feature.Scenarios) == 0 {
		return nil
	}
	return append([]Step{}, feature.Scenarios[0].Given...)
}

func featureReadyKey(slug string) string {
	return slugifyDotted(slug)
}

func slugifyDotted(slug string) string {
	out := ""
	lastDot := false
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out += string(r)
			lastDot = false
		} else if !lastDot {
			out += "."
			lastDot = true
		}
	}
	return strings.Trim(out, ".")
}

func expandDimensions(dims []Dimension) []map[string]string {
	if len(dims) == 0 {
		return []map[string]string{{}}
	}
	rest := expandDimensions(dims[1:])
	out := []map[string]string{}
	for _, val := range dims[0].Values {
		for _, r := range rest {
			m := map[string]string{dims[0].Name: val}
			for k, v := range r {
				m[k] = v
			}
			out = append(out, m)
		}
	}
	return out
}

func ValidatePropertyDimensions(prop Property) error {
	for _, d := range prop.Dimensions {
		if d.Name != "viewport" && d.Name != "route" {
			return fmt.Errorf(
				"property %s has unsupported dimension %s",
				prop.Slug,
				d.Name,
			)
		}
		if len(d.Values) == 0 {
			return fmt.Errorf("property %s dimension %s has no values", prop.Slug, d.Name)
		}
	}
	return nil
}
