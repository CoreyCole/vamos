package workspaces

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type ImplWorkspaceGitState struct {
	Branch       string
	Commit       string
	TrunkBranch  string
	TopBranch    string
	BottomBranch string
	BottomParent string
	BaseBranch   string
	AheadCount   int
	BehindCount  int
	Merged       bool
	MergeRef     string
	Available    bool
	Detail       string
}

func InspectImplWorkspaceGit(
	ctx context.Context,
	checkoutPath, trunkBranch string,
) ImplWorkspaceGitState {
	stack := InspectStackWithTrunk(ctx, checkoutPath, trunkBranch)
	if strings.TrimSpace(stack.TrunkBranch) == "" {
		stack.TrunkBranch = firstNonEmpty(trunkBranch, "main")
	}
	if strings.TrimSpace(stack.TopBranch) == "" {
		stack.TopBranch = stack.Branch
	}
	if strings.TrimSpace(stack.BottomBranch) == "" {
		stack.BottomBranch = stack.Branch
	}
	return ImplWorkspaceGitState{
		Branch:       stack.Branch,
		Commit:       gitCommit(ctx, checkoutPath),
		TrunkBranch:  stack.TrunkBranch,
		TopBranch:    stack.TopBranch,
		BottomBranch: stack.BottomBranch,
		BottomParent: stack.BottomParent,
		BaseBranch:   stack.BaseBranch,
		AheadCount:   stack.AheadCount,
		BehindCount:  stack.BehindCount,
		Merged:       stack.Merged,
		MergeRef:     stack.MergeRef,
		Available:    stack.Available,
		Detail:       stack.Detail,
	}
}

func DetermineMissingWorkspaceStatus(
	ctx context.Context,
	mainCheckoutPath string,
	stored db.ImplWorkspace,
) (ImplWorkspaceStatus, string) {
	commit := nullStringValue(stored.CommitHash)
	trunk := firstNonEmpty(nullStringValue(stored.TrunkBranch), "origin/main", "main")
	if commit == "" || strings.TrimSpace(mainCheckoutPath) == "" {
		return ImplWorkspaceStatusCleanedUp, "missing checkout; no commit evidence"
	}
	ref := trunk
	if !strings.HasPrefix(ref, "origin/") && ref != "main" {
		ref = "origin/" + ref
	}
	if err := runCheckoutCommandNoOutput(
		ctx,
		mainCheckoutPath,
		"git",
		"merge-base",
		"--is-ancestor",
		commit,
		ref,
	); err == nil {
		return ImplWorkspaceStatusMerged,
			fmt.Sprintf("commit %s is ancestor of %s", commit, ref)
	}
	if err := runCheckoutCommandNoOutput(
		ctx,
		mainCheckoutPath,
		"git",
		"merge-base",
		"--is-ancestor",
		commit,
		"main",
	); err == nil {
		return ImplWorkspaceStatusMerged,
			fmt.Sprintf("commit %s is ancestor of main", commit)
	}
	return ImplWorkspaceStatusCleanedUp, "missing checkout; commit not proven merged"
}

func gitCommit(ctx context.Context, checkoutPath string) string {
	out, err := runCheckoutCommand(
		ctx,
		checkoutPath,
		"git",
		"rev-parse",
		"--short",
		"HEAD",
	)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}
