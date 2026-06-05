package server

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
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

type ProjectCheckoutResolution struct {
	ProjectID    string
	Repo         RepoConfig
	CheckoutName string
	Checkout     CheckoutConfig
	RootPath     string
}

func ResolveProjectCheckout(projects ProjectsConfig, projectID string) (ProjectCheckoutResolution, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectCheckoutResolution{}, fmt.Errorf("project_id is required")
	}
	repo, ok := projects.Repos[projectID]
	if !ok {
		return ProjectCheckoutResolution{}, fmt.Errorf("project %q not configured", projectID)
	}
	checkoutName := strings.TrimSpace(repo.DefaultCheckout)
	if checkoutName == "" {
		checkoutName = strings.TrimSpace(projects.DefaultCheckout)
	}
	if checkoutName == "" {
		checkoutName = firstInteractiveProjectCheckoutName(projects, repo)
	}
	if checkoutName == "" {
		return ProjectCheckoutResolution{}, fmt.Errorf("project %q has no interactive working checkout", projectID)
	}
	checkout, ok := repo.Checkouts[checkoutName]
	if !ok {
		return ProjectCheckoutResolution{}, fmt.Errorf("checkout %q for project %q not found", checkoutName, projectID)
	}
	if !isInteractiveProjectCheckout(projects, repo, checkoutName, checkout) {
		return ProjectCheckoutResolution{}, fmt.Errorf("checkout %q for project %q is not an interactive working checkout", checkoutName, projectID)
	}
	if strings.TrimSpace(checkout.RootPath) == "" {
		return ProjectCheckoutResolution{}, fmt.Errorf("checkout %q for project %q has no root_path", checkoutName, projectID)
	}
	return ProjectCheckoutResolution{
		ProjectID:    projectID,
		Repo:         repo,
		CheckoutName: checkoutName,
		Checkout:     checkout,
		RootPath:     checkout.RootPath,
	}, nil
}

func firstInteractiveProjectCheckoutName(projects ProjectsConfig, repo RepoConfig) string {
	keys := make([]string, 0, len(repo.Checkouts))
	for key := range repo.Checkouts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if isInteractiveProjectCheckout(projects, repo, key, repo.Checkouts[key]) {
			return key
		}
	}
	return ""
}

func isInteractiveProjectCheckout(projects ProjectsConfig, repo RepoConfig, checkoutName string, checkout CheckoutConfig) bool {
	if !isInteractiveCheckout(checkoutName, checkout) {
		return false
	}
	checkoutName = strings.TrimSpace(checkoutName)
	if checkoutName == "" {
		return false
	}
	if checkoutName == strings.TrimSpace(repo.BaselineCheckout) {
		return false
	}
	if checkoutName == strings.TrimSpace(projects.DefaultBaselineCheckout) {
		return false
	}
	return true
}

func isInteractiveCheckout(checkoutName string, checkout CheckoutConfig) bool {
	if strings.TrimSpace(checkoutName) == "" || strings.TrimSpace(checkout.RootPath) == "" {
		return false
	}
	if checkout.Role == CheckoutRoleMain || checkout.MustBeClean {
		return false
	}
	return true
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
