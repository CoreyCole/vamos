package repair

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

var allowedScopes = map[FixScope]bool{
	FixScopeSelectors: true,
	FixScopeSteps:     true,
	FixScopeRuntime:   true,
	FixScopeGenerated: true,
}

var forbiddenPathPrefixes = []string{
	"docs/features",
	"server",
	"static",
	"pkg/components",
}

var allowedPathPrefixesByScope = map[FixScope][]string{
	FixScopeSelectors: {"pkg/e2e/selectors"},
	FixScopeSteps:     {"pkg/e2e/steps"},
	FixScopeRuntime:   {"pkg/e2e/runtime"},
	FixScopeGenerated: {"pkg/e2e/generated"},
}

func ValidatePlan(plan Plan) error {
	for _, change := range plan.Changes {
		if !allowedScopes[change.Scope] {
			return fmt.Errorf("scope %s is not allowed", change.Scope)
		}
		path := cleanPlanPath(change.Path)
		if path == "." {
			return fmt.Errorf("repair change missing path")
		}
		for _, prefix := range forbiddenPathPrefixes {
			if hasPathPrefix(path, prefix) {
				return fmt.Errorf(
					"repair may not edit %s without human approval",
					change.Path,
				)
			}
		}
		if !scopeAllowsPath(change.Scope, path) {
			return fmt.Errorf(
				"repair scope %s may not edit %s without human approval",
				change.Scope,
				change.Path,
			)
		}
	}
	return nil
}

func scopeAllowsPath(scope FixScope, path string) bool {
	for _, prefix := range allowedPathPrefixesByScope[scope] {
		if hasPathPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func hasPathPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func ApplyPlan(ctx context.Context, plan Plan) error {
	return fmt.Errorf(
		"automatic repair apply is not enabled; inspect plan and implement bounded changes manually",
	)
}

func cleanPlanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "."
	}
	return filepath.ToSlash(filepath.Clean(path))
}
