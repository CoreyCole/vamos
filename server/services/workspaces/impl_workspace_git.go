package workspaces

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

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
	MergeProof   MergeProof
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
	proof := InspectMergeProof(ctx, checkoutPath, stack.TrunkBranch)
	stack.Merged = proof.Kind == MergeProofAncestor || proof.Kind == MergeProofPatchEquivalent
	if stack.Merged {
		stack.MergeRef = firstNonEmpty(proof.SourceRef, stack.MergeRef)
	} else {
		stack.MergeRef = ""
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
		MergeProof:   proof,
		Available:    stack.Available,
		Detail:       stack.Detail,
	}
}

func DetermineMissingWorkspaceStatus(
	ctx context.Context,
	mainCheckoutPath string,
	stored db.ImplWorkspace,
) (ImplWorkspaceStatus, MergeProof) {
	commit := nullStringValue(stored.CommitHash)
	ref := mergeTruthRef(firstNonEmpty(nullStringValue(stored.TrunkBranch), "main"))
	if commit == "" {
		return ImplWorkspaceStatusActive, MergeProof{Kind: MergeProofUnknown, SourceRef: ref, RiskReason: "missing checkout; no commit evidence"}
	}
	if strings.TrimSpace(mainCheckoutPath) == "" {
		return ImplWorkspaceStatusActive, MergeProof{Kind: MergeProofUnknown, SourceRef: ref, RiskReason: "missing checkout; no main checkout for proof"}
	}
	_ = fetchOriginMain(ctx, mainCheckoutPath)
	target := revParseRef(ctx, mainCheckoutPath, ref)
	if err := runCheckoutCommandNoOutput(
		ctx,
		mainCheckoutPath,
		"git",
		"merge-base",
		"--is-ancestor",
		commit,
		ref,
	); err == nil {
		return ImplWorkspaceStatusMerged, MergeProof{Kind: MergeProofAncestor, SourceRef: ref, TargetCommit: target, ProvenAt: time.Now(), Detail: fmt.Sprintf("commit %s is ancestor of %s", commit, ref)}
	}
	return ImplWorkspaceStatusActive, MergeProof{Kind: MergeProofUnknown, SourceRef: ref, TargetCommit: target, RiskReason: "missing checkout; commit not proven merged"}
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
