package workspaces

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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
	targets ...[]MergeProofTarget,
) ImplWorkspaceGitState {
	var mergeTargets []MergeProofTarget
	if len(targets) > 0 {
		mergeTargets = targets[0]
	}
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
	var proof MergeProof
	if len(mergeTargets) > 0 {
		proof = InspectMergeProofMulti(ctx, checkoutPath, mergeTargets)
	} else {
		proof = InspectMergeProof(ctx, checkoutPath, stack.TrunkBranch)
	}
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

// BuildMergeProofTargets builds the once-per-Sync ordered tip list:
// origin/main, checkout:main, checkout:stage, then active feature peer HEADs (newer/longer first).
func BuildMergeProofTargets(ctx context.Context, discovered []Workspace) []MergeProofTarget {
	var liveCheckout string
	var mainWS, stageWS *Workspace
	peers := make([]Workspace, 0, len(discovered))

	for i := range discovered {
		ws := discovered[i]
		if ws.Status == StatusInvalid || strings.TrimSpace(ws.CheckoutPath) == "" {
			continue
		}
		if liveCheckout == "" {
			liveCheckout = ws.CheckoutPath
		}
		switch {
		case ws.IsMain || ws.CheckoutRole == CheckoutRoleMain || ws.Slug == mainWorkspaceSlug:
			copy := ws
			mainWS = &copy
		case ws.CheckoutRole == CheckoutRoleStage || ws.Slug == "stage":
			copy := ws
			stageWS = &copy
		default:
			peers = append(peers, ws)
		}
	}

	targets := make([]MergeProofTarget, 0, 3+len(peers))

	originPath := liveCheckout
	if mainWS != nil && strings.TrimSpace(mainWS.CheckoutPath) != "" {
		originPath = mainWS.CheckoutPath
	}
	if originPath != "" {
		_ = fetchOriginMain(ctx, originPath)
		if tip := strings.TrimSpace(revParseRef(ctx, originPath, "origin/main")); tip != "" {
			targets = append(targets, MergeProofTarget{
				SourceRef:    "origin/main",
				Commit:       tip,
				CheckoutPath: originPath,
				GitRef:       "origin/main",
			})
		}
	}

	if mainWS != nil {
		tip := strings.TrimSpace(firstNonEmpty(mainWS.Commit, gitCommit(ctx, mainWS.CheckoutPath)))
		if tip != "" {
			targets = append(targets, MergeProofTarget{
				SourceRef:    "checkout:main",
				Commit:       tip,
				CheckoutPath: mainWS.CheckoutPath,
				GitRef:       "HEAD",
			})
		}
	}
	if stageWS != nil {
		tip := strings.TrimSpace(firstNonEmpty(stageWS.Commit, gitCommit(ctx, stageWS.CheckoutPath)))
		if tip != "" {
			targets = append(targets, MergeProofTarget{
				SourceRef:    "checkout:stage",
				Commit:       tip,
				CheckoutPath: stageWS.CheckoutPath,
				GitRef:       "HEAD",
			})
		}
	}

	type peerTip struct {
		ws     Workspace
		commit string
		when   int64
		length int
	}
	peerTips := make([]peerTip, 0, len(peers))
	for _, ws := range peers {
		commit := strings.TrimSpace(firstNonEmpty(ws.Commit, gitCommit(ctx, ws.CheckoutPath)))
		if commit == "" {
			continue
		}
		peerTips = append(peerTips, peerTip{
			ws:     ws,
			commit: commit,
			when:   commitUnixTime(ctx, ws.CheckoutPath, commit),
			length: tipLength(ctx, ws.CheckoutPath, commit),
		})
	}
	sort.SliceStable(peerTips, func(i, j int) bool {
		if peerTips[i].when != peerTips[j].when {
			return peerTips[i].when > peerTips[j].when
		}
		if peerTips[i].length != peerTips[j].length {
			return peerTips[i].length > peerTips[j].length
		}
		return peerTips[i].ws.Slug < peerTips[j].ws.Slug
	})
	for _, peer := range peerTips {
		targets = append(targets, MergeProofTarget{
			SourceRef:    fmt.Sprintf("peer:%s@%s", peer.ws.Slug, peer.commit),
			Commit:       peer.commit,
			CheckoutPath: peer.ws.CheckoutPath,
			GitRef:       "HEAD",
		})
	}
	return targets
}

// MergeProofTargetsForSubject drops the subject's own peer tip so self is never a strong hit.
func MergeProofTargetsForSubject(targets []MergeProofTarget, subjectSlug, subjectCheckout string) []MergeProofTarget {
	subjectSlug = strings.TrimSpace(subjectSlug)
	subjectCheckout = cleanPathKey(subjectCheckout)
	prefix := "peer:" + subjectSlug + "@"
	out := make([]MergeProofTarget, 0, len(targets))
	for _, target := range targets {
		if subjectSlug != "" && strings.HasPrefix(target.SourceRef, prefix) {
			continue
		}
		if subjectCheckout != "" && cleanPathKey(target.CheckoutPath) == subjectCheckout &&
			strings.HasPrefix(target.SourceRef, "peer:") {
			continue
		}
		out = append(out, target)
	}
	return out
}

func DetermineMissingWorkspaceStatus(
	ctx context.Context,
	mainCheckoutPath string,
	stored db.ImplWorkspace,
	targets ...[]MergeProofTarget,
) (ImplWorkspaceStatus, MergeProof) {
	commit := nullStringValue(stored.CommitHash)
	var mergeTargets []MergeProofTarget
	if len(targets) > 0 {
		mergeTargets = MergeProofTargetsForSubject(targets[0], stored.WorkspaceSlug, stored.CheckoutPath)
	}
	if len(mergeTargets) > 0 {
		if commit == "" {
			return ImplWorkspaceStatusActive, MergeProof{
				Kind:       MergeProofUnknown,
				RiskReason: "missing checkout; no commit evidence",
			}
		}
		proof := inspectMissingCommitAgainstTargets(ctx, commit, mergeTargets)
		if proof.Strong() {
			return ImplWorkspaceStatusMerged, proof
		}
		proof.Kind = MergeProofUnknown
		proof.RiskReason = "missing checkout; commit not proven merged"
		return ImplWorkspaceStatusActive, proof
	}

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

func inspectMissingCommitAgainstTargets(
	ctx context.Context,
	commit string,
	targets []MergeProofTarget,
) MergeProof {
	commit = strings.TrimSpace(commit)
	var last MergeProof
	for _, target := range targets {
		sourceRef := strings.TrimSpace(target.SourceRef)
		if sourceRef == "" || isBareLocalBranchMergeProofRef(sourceRef) {
			continue
		}
		tipCheckout := strings.TrimSpace(target.CheckoutPath)
		tipCommit := strings.TrimSpace(firstNonEmpty(target.Commit, target.GitRef))
		if tipCheckout == "" {
			continue
		}
		if tipCommit == "" || tipCommit == "HEAD" {
			tipCommit = strings.TrimSpace(revParseRef(ctx, tipCheckout, firstNonEmpty(target.GitRef, "HEAD")))
		}
		if tipCommit == "" {
			continue
		}
		checkRef := tipCommit
		if sourceRef == "origin/main" || strings.HasPrefix(sourceRef, "origin/") {
			_ = fetchOriginMain(ctx, tipCheckout)
			if refTip := strings.TrimSpace(revParseRef(ctx, tipCheckout, sourceRef)); refTip != "" {
				tipCommit = refTip
			}
			checkRef = firstNonEmpty(sourceRef, tipCommit)
		}
		if err := runCheckoutCommandNoOutput(ctx, tipCheckout, "git", "merge-base", "--is-ancestor", commit, checkRef); err == nil {
			return MergeProof{
				Kind:         MergeProofAncestor,
				SourceRef:    sourceRef,
				TargetCommit: tipCommit,
				ProvenAt:     time.Now(),
				Detail:       fmt.Sprintf("commit %s is ancestor of %s", commit, sourceRef),
			}
		}
		if provePatchEquivalentCommits(ctx, tipCheckout, tipCommit, commit) {
			return MergeProof{
				Kind:         MergeProofPatchEquivalent,
				SourceRef:    sourceRef,
				TargetCommit: tipCommit,
				ProvenAt:     time.Now(),
				Detail:       fmt.Sprintf("commit %s is patch-equivalent to %s", commit, sourceRef),
			}
		}
		last = MergeProof{
			Kind:         MergeProofUnknown,
			SourceRef:    sourceRef,
			TargetCommit: tipCommit,
			RiskReason:   "not ancestor of any target",
		}
	}
	if last.Kind == "" {
		last = MergeProof{Kind: MergeProofUnknown, RiskReason: "not ancestor of any target"}
	}
	return last
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

func commitUnixTime(ctx context.Context, checkoutPath, commit string) int64 {
	out, err := runCheckoutCommand(ctx, checkoutPath, "git", "log", "-1", "--format=%ct", commit)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return n
}

func tipLength(ctx context.Context, checkoutPath, commit string) int {
	out, err := runCheckoutCommand(ctx, checkoutPath, "git", "rev-list", "--count", commit)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

// mergeProofKeeperTokens are configured dogfood keepers that must never leave active via merge proof.
func mergeProofKeeperTokens() []string {
	return []string{
		"agent-memory",
		"rebalancer",
		"datastar-sse-ui-declaration",
	}
}

func isMergeProofKeeperWorkspace(ws Workspace) bool {
	if IsProtectedCheckoutRole(ws.CheckoutRole) || ws.IsMain || ws.Slug == mainWorkspaceSlug || ws.Slug == "stage" {
		return true
	}
	return matchesMergeProofKeeperToken(ws.Slug, ws.CheckoutPath, ws.DisplayName)
}

func matchesMergeProofKeeperToken(slug, checkoutPath, displayName string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(slug),
		filepath.Base(strings.TrimSpace(checkoutPath)),
		strings.TrimSpace(displayName),
	}, "\n"))
	for _, token := range mergeProofKeeperTokens() {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" && strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}
