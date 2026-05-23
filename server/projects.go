package server

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultWorkingDir returns the configured working checkout for new interactive work.
func DefaultWorkingDir(projects ProjectsConfig) (string, error) {
	repoName := strings.TrimSpace(projects.DefaultRepo)
	if repoName == "" {
		return "", nil
	}
	repo, ok := projects.Repos[repoName]
	if !ok {
		return "", fmt.Errorf("projects.default_repo %q not found", repoName)
	}
	checkoutName := strings.TrimSpace(repo.DefaultCheckout)
	if checkoutName == "" {
		checkoutName = strings.TrimSpace(projects.DefaultCheckout)
	}
	if checkoutName == "" {
		return "", nil
	}
	checkout, ok := repo.Checkouts[checkoutName]
	if !ok {
		return "", fmt.Errorf(
			"default checkout %q for repo %q not found",
			checkoutName,
			repoName,
		)
	}
	return checkout.RootPath, nil
}

// BaselineBranch returns the branch a baseline checkout should track.
func BaselineBranch(repo RepoConfig, checkout CheckoutConfig) string {
	if branch := strings.TrimSpace(checkout.WebhookSyncBranch); branch != "" {
		return branch
	}
	if branch := strings.TrimSpace(repo.DefaultBranch); branch != "" {
		return branch
	}
	return "main"
}

// BaselineCheckout returns the configured clean/latest checkout for repoName.
func BaselineCheckout(
	projects ProjectsConfig,
	repoName string,
) (RepoConfig, string, CheckoutConfig, error) {
	repo, ok := projects.Repos[repoName]
	if !ok {
		return RepoConfig{}, "", CheckoutConfig{}, fmt.Errorf(
			"repo %q not configured",
			repoName,
		)
	}
	name := strings.TrimSpace(repo.BaselineCheckout)
	if name == "" {
		name = strings.TrimSpace(projects.DefaultBaselineCheckout)
	}
	if name == "" {
		return repo, "", CheckoutConfig{}, fmt.Errorf(
			"repo %q has no baseline_checkout",
			repoName,
		)
	}
	checkout, ok := repo.Checkouts[name]
	if !ok {
		return repo, name, CheckoutConfig{}, fmt.Errorf(
			"baseline checkout %q for repo %q not found",
			name,
			repoName,
		)
	}
	return repo, name, checkout, nil
}

// ValidateBaselineCheckouts validates local-only baseline invariants. Latest/upstream
// checks are intentionally left to workspace copy/sync paths so startup can run offline.
func ValidateBaselineCheckouts(ctx context.Context, projects ProjectsConfig) error {
	for repoName, repo := range projects.Repos {
		baselineName := strings.TrimSpace(repo.BaselineCheckout)
		if baselineName == "" {
			continue
		}
		checkout, ok := repo.Checkouts[baselineName]
		if !ok {
			return fmt.Errorf(
				"repos.%s.baseline_checkout %q not found",
				repoName,
				baselineName,
			)
		}
		if !checkout.MustBeClean {
			continue
		}
		out, err := exec.CommandContext(ctx, "git", "-C", checkout.RootPath, "status", "--short", "--untracked-files=all").
			Output()
		if err != nil {
			return fmt.Errorf("validate baseline %s: %w", repoName, err)
		}
		if strings.TrimSpace(string(out)) != "" {
			return fmt.Errorf(
				"baseline checkout %s (%s) is dirty",
				baselineName,
				checkout.RootPath,
			)
		}
	}
	return nil
}
