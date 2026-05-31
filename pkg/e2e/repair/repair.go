package repair

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type FixScope string

const (
	FixScopeDatastarUI   FixScope = "datastarui"
	FixScopeVamosHelpers FixScope = "vamos_helpers"
	FixScopeTests        FixScope = "tests"
)

type Request struct {
	RunManifestPath string
	FailuresPath    string
	AllowedScopes   []FixScope
}

type Failure struct {
	Story    string `json:"Story,omitempty"`
	Scenario string `json:"Scenario,omitempty"`
	Viewport string `json:"Viewport,omitempty"`
	Step     string `json:"Step,omitempty"`
	Error    string `json:"Error,omitempty"`
}

type Plan struct {
	Changes    []Change `json:"changes,omitempty"`
	NeedsHuman []string `json:"needsHuman,omitempty"`
}

type Change struct {
	Scope FixScope `json:"scope"`
	Path  string   `json:"path"`
	Why   string   `json:"why"`
}

func BuildPlan(ctx context.Context, req Request) (Plan, error) {
	if strings.TrimSpace(req.FailuresPath) == "" {
		return Plan{}, fmt.Errorf("failures path is required")
	}
	data, err := os.ReadFile(req.FailuresPath)
	if err != nil {
		return Plan{}, err
	}
	failures, err := decodeFailures(data)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{}
	for _, failure := range failures {
		selectChangeForFailure(&plan, failure)
	}
	if len(plan.Changes) == 0 && len(plan.NeedsHuman) == 0 {
		plan.NeedsHuman = append(
			plan.NeedsHuman,
			"no concrete failures found; inspect run artifacts before changing code",
		)
	}
	return filterPlanScopes(plan, req.AllowedScopes), nil
}

func decodeFailures(data []byte) ([]Failure, error) {
	var failures []Failure
	if err := json.Unmarshal(data, &failures); err == nil {
		return failures, nil
	}
	var generic []map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, err
	}
	failures = make([]Failure, 0, len(generic))
	for _, item := range generic {
		failures = append(failures, Failure{
			Story:    fmt.Sprint(item["Story"]),
			Scenario: fmt.Sprint(item["Scenario"]),
			Viewport: fmt.Sprint(item["Viewport"]),
			Step:     fmt.Sprint(item["Step"]),
			Error:    fmt.Sprint(item["Error"]),
		})
	}
	return failures, nil
}

func selectChangeForFailure(plan *Plan, failure Failure) {
	msg := strings.ToLower(strings.Join([]string{failure.Step, failure.Error}, " "))
	switch {
	case strings.Contains(msg, "unsupported story step") || strings.Contains(msg, "unknown fixture"):
		plan.NeedsHuman = append(
			plan.NeedsHuman,
			"new story language or fixture requires explicit story/step design",
		)
	case strings.Contains(msg, "go story") || strings.Contains(msg, "test"):
		addChange(
			plan,
			FixScopeTests,
			"pkg/e2e/tests",
			"authored Go Story test needs bounded repair",
		)
	case strings.Contains(msg, "unknown selector") || strings.Contains(msg, "not visible") || strings.Contains(msg, "locator") || strings.Contains(msg, "selector"):
		addChange(
			plan,
			FixScopeVamosHelpers,
			"pkg/e2e/vamos",
			"typed Vamos locator/readiness helper drift",
		)
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "networkidle") || strings.Contains(msg, "browser") || strings.Contains(msg, "artifact"):
		addChange(
			plan,
			FixScopeDatastarUI,
			"../datastarui/e2e",
			"generic DatastarUI runtime/artifact behavior needs repair",
		)
	case strings.Contains(msg, "step") || strings.Contains(msg, "expected"):
		addChange(
			plan,
			FixScopeVamosHelpers,
			"pkg/e2e/vamos",
			"typed Vamos action/expectation helper needs repair",
		)
	default:
		addChange(
			plan,
			FixScopeVamosHelpers,
			"pkg/e2e/vamos",
			"app E2E helper failure needs bounded repair",
		)
	}
}

func addChange(plan *Plan, scope FixScope, path, why string) {
	for _, change := range plan.Changes {
		if change.Scope == scope && change.Path == path {
			return
		}
	}
	plan.Changes = append(plan.Changes, Change{Scope: scope, Path: path, Why: why})
}

func filterPlanScopes(plan Plan, allowed []FixScope) Plan {
	if len(allowed) == 0 {
		return plan
	}
	allowedSet := map[FixScope]bool{}
	for _, scope := range allowed {
		allowedSet[scope] = true
	}
	filtered := Plan{NeedsHuman: append([]string{}, plan.NeedsHuman...)}
	for _, change := range plan.Changes {
		if allowedSet[change.Scope] {
			filtered.Changes = append(filtered.Changes, change)
			continue
		}
		filtered.NeedsHuman = append(
			filtered.NeedsHuman,
			fmt.Sprintf(
				"scope %s for %s is outside allowed repair scopes",
				change.Scope,
				change.Path,
			),
		)
	}
	return filtered
}
