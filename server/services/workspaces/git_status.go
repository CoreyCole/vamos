package workspaces

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/collections"
)

type StackSummary struct {
	Branch       string
	TopBranch    string
	BottomBranch string
	BottomParent string
	TrunkBranch  string
	BaseBranch   string
	AheadCount   int
	BehindCount  int
	Merged       bool
	MergeRef     string
	Available    bool
	Detail       string
}

func InspectStack(ctx context.Context, checkoutPath string) StackSummary {
	return InspectStackWithTrunk(ctx, checkoutPath, "main")
}

func InspectStackWithTrunk(
	ctx context.Context,
	checkoutPath, trunkBranch string,
) StackSummary {
	checkoutPath = strings.TrimSpace(checkoutPath)
	if checkoutPath == "" {
		return StackSummary{Available: false, Detail: "checkout path is empty"}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var details []string
	summary := StackSummary{}
	if branch, err := runCheckoutCommand(
		ctx,
		checkoutPath,
		"git",
		"branch",
		"--show-current",
	); err == nil {
		summary.Branch = strings.TrimSpace(branch)
	} else {
		details = append(details, "git branch: "+err.Error())
	}
	if base, err := runCheckoutCommand(
		ctx,
		checkoutPath,
		"git",
		"rev-parse",
		"--abbrev-ref",
		"--symbolic-full-name",
		"@{u}",
	); err == nil {
		summary.BaseBranch = strings.TrimSpace(base)
		if counts, err := runCheckoutCommand(
			ctx,
			checkoutPath,
			"git",
			"rev-list",
			"--left-right",
			"--count",
			"@{u}...HEAD",
		); err == nil {
			fields := strings.Fields(counts)
			if len(fields) == 2 {
				summary.BehindCount, _ = strconv.Atoi(fields[0])
				summary.AheadCount, _ = strconv.Atoi(fields[1])
			}
		}
	} else {
		details = append(details, "upstream: "+err.Error())
	}
	summary.Merged, summary.MergeRef = inspectCheckoutHeadMerged(
		ctx,
		checkoutPath,
		trunkBranch,
	)
	if out, err := runCheckoutCommand(ctx, checkoutPath, "gt", "trunk"); err == nil {
		summary.TrunkBranch = firstNonEmptyLine(out)
	} else {
		details = append(details, "gt trunk: "+err.Error())
	}
	if out, err := runCheckoutCommand(ctx, checkoutPath, "gt", "parent"); err == nil {
		summary.BottomParent = firstNonEmptyLine(out)
	} else {
		details = append(details, "gt parent: "+err.Error())
	}
	if out, err := runCheckoutCommand(
		ctx,
		checkoutPath,
		"gt",
		"log",
		"short",
	); err == nil {
		summary.TopBranch, summary.BottomBranch = graphiteStackBranches(
			out,
			summary.TrunkBranch,
			summary.Branch,
		)
	} else {
		details = append(details, "gt log: "+err.Error())
	}
	if summary.Branch != "" {
		summary.Available = len(details) == 0 ||
			!strings.Contains(strings.Join(details, "; "), "git branch:")
	}
	if len(details) > 0 {
		summary.Detail = "stack unavailable: " + strings.Join(details, "; ")
	} else {
		summary.Detail = "stack available"
	}
	return summary
}

func checkoutMergeRefCandidates(trunkBranch string) []string {
	trunkBranch = strings.TrimSpace(trunkBranch)
	if trunkBranch == "" {
		trunkBranch = "main"
	}
	seen := collections.NewSet[string]()
	refs := make([]string, 0, 4)
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen.Has(ref) {
			return
		}
		seen.Add(ref)
		refs = append(refs, ref)
	}

	if strings.HasPrefix(trunkBranch, "origin/") {
		add(trunkBranch)
		add(strings.TrimPrefix(trunkBranch, "origin/"))
	} else {
		add("origin/" + trunkBranch)
		add(trunkBranch)
	}
	add("origin/main")
	add("main")
	return refs
}

func inspectCheckoutHeadMerged(
	ctx context.Context,
	checkoutPath, trunkBranch string,
) (bool, string) {
	for _, ref := range checkoutMergeRefCandidates(trunkBranch) {
		if err := runCheckoutCommandNoOutput(
			ctx,
			checkoutPath,
			"git",
			"merge-base",
			"--is-ancestor",
			"HEAD",
			ref,
		); err == nil {
			return true, ref
		}
	}
	return false, ""
}

func runCheckoutCommand(
	ctx context.Context,
	dir, name string,
	args ...string,
) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", commandError(err, out)
	}
	return string(out), nil
}

func runCheckoutCommandNoOutput(
	ctx context.Context,
	dir, name string,
	args ...string,
) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return commandError(err, out)
	}
	return nil
}

func commandError(err error, out []byte) error {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err
	}
	return errors.New(err.Error() + ": " + text)
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func graphiteStackBranches(logOutput, trunk, current string) (top, bottom string) {
	trunk = strings.TrimSpace(trunk)
	current = strings.TrimSpace(current)
	branches := graphiteLogBranches(logOutput)
	if len(branches) == 0 {
		return current, current
	}

	top = branches[0]
	for i := len(branches) - 1; i >= 0; i-- {
		branch := branches[i]
		if branch != "" && branch != trunk {
			bottom = branch
			break
		}
	}
	if bottom == "" {
		bottom = top
	}
	return top, bottom
}

func graphiteLogBranches(logOutput string) []string {
	var branches []string
	for _, line := range strings.Split(logOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		trimmed = strings.Split(trimmed, " (")[0]
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		branch := strings.Trim(fields[len(fields)-1], "()")
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches
}
