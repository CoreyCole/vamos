package workspaces

import (
	"context"
	"strconv"
	"strings"
	"time"
)

type GitInspector interface {
	Head(ctx context.Context, checkout string) (string, error)
	IsClean(ctx context.Context, checkout string) (bool, string, error)
	IsAncestor(ctx context.Context, checkout string, ancestor, descendant string) (bool, error)
	AheadBehind(ctx context.Context, checkout string, left, right string) (ahead, behind int, err error)
}

type ShellGitInspector struct {
	Timeout time.Duration
}

func (g ShellGitInspector) Head(ctx context.Context, checkout string) (string, error) {
	out, err := g.run(ctx, checkout, "git", "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}

func (g ShellGitInspector) IsClean(ctx context.Context, checkout string) (bool, string, error) {
	out, err := g.run(ctx, checkout, "git", "status", "--porcelain")
	if err != nil {
		return false, "", err
	}
	detail := strings.TrimSpace(out)
	return detail == "", detail, nil
}

func (g ShellGitInspector) IsAncestor(ctx context.Context, checkout string, ancestor, descendant string) (bool, error) {
	err := g.runNoOutput(ctx, checkout, "git", "merge-base", "--is-ancestor", ancestor, descendant)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (g ShellGitInspector) AheadBehind(ctx context.Context, checkout string, left, right string) (ahead, behind int, err error) {
	out, err := g.run(ctx, checkout, "git", "rev-list", "--left-right", "--count", left+"..."+right)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, nil
	}
	ahead, _ = strconv.Atoi(fields[0])
	behind, _ = strconv.Atoi(fields[1])
	return ahead, behind, nil
}

func (g ShellGitInspector) run(ctx context.Context, checkout, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout())
	defer cancel()
	return runCheckoutCommand(ctx, checkout, name, args...)
}

func (g ShellGitInspector) runNoOutput(ctx context.Context, checkout, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, g.timeout())
	defer cancel()
	return runCheckoutCommandNoOutput(ctx, checkout, name, args...)
}

func (g ShellGitInspector) timeout() time.Duration {
	if g.Timeout > 0 {
		return g.Timeout
	}
	return 5 * time.Second
}
