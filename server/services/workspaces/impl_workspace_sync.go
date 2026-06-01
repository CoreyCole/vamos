package workspaces

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/collections"
	"github.com/CoreyCole/vamos/pkg/db"
)

type ImplWorkspaceStatus string

const (
	ImplWorkspaceStatusActive    ImplWorkspaceStatus = "active"
	ImplWorkspaceStatusCleanedUp ImplWorkspaceStatus = "cleaned_up"
	ImplWorkspaceStatusMerged    ImplWorkspaceStatus = "merged"
)

type ImplWorkspaceEnvAction string

const (
	ImplWorkspaceEnvActionNone     ImplWorkspaceEnvAction = "none"
	ImplWorkspaceEnvActionCreated  ImplWorkspaceEnvAction = "created"
	ImplWorkspaceEnvActionRepaired ImplWorkspaceEnvAction = "repaired"
	ImplWorkspaceEnvActionSkipped  ImplWorkspaceEnvAction = "skipped"
)

type ImplWorkspaceSyncer struct {
	Queries *db.Queries
	Now     func() time.Time
}

type ImplWorkspaceSyncInput struct {
	ProjectID    string
	Discovery    ImplWorkspaceDiscoveryConfig
	ManagerURL   string
	RestartToken string
	TrunkBranch  string
}

type ImplWorkspaceSyncResult struct {
	Scanned     int
	Discovered  int
	Upserted    int
	RepairedEnv int
	CleanedUp   int
	Merged      int
	Changed     bool
}

func (s *ImplWorkspaceSyncer) Sync(
	ctx context.Context,
	input ImplWorkspaceSyncInput,
) (ImplWorkspaceSyncResult, error) {
	if s == nil || s.Queries == nil {
		return ImplWorkspaceSyncResult{}, errors.New(
			"impl workspace syncer requires queries",
		)
	}
	discoveryProjectID := firstNonEmpty(input.Discovery.ProjectID, input.ProjectID)
	discovered, err := Discover(DiscoveryConfig{
		ProjectID:           discoveryProjectID,
		MainCheckoutPath:    input.Discovery.MainCheckoutPath,
		ParentDir:           input.Discovery.ParentDir,
		Domain:              input.Discovery.Domain,
		MetadataDirName:     input.Discovery.MetadataDirName,
		CheckoutPrefixes:    input.Discovery.CheckoutPrefixes,
		MainCheckoutName:    input.Discovery.MainCheckoutName,
		ModuleMarker:        input.Discovery.ModuleMarker,
		PackageSubdir:       input.Discovery.PackageSubdir,
		ConfiguredCheckouts: input.Discovery.ConfiguredCheckouts,
	})
	if err != nil {
		return ImplWorkspaceSyncResult{}, err
	}

	result := ImplWorkspaceSyncResult{
		Scanned:    len(discovered),
		Discovered: len(discovered),
	}
	seen := collections.NewSet[string]()
	for _, ws := range discovered {
		slug := strings.TrimSpace(ws.Slug)
		projectID := firstNonEmpty(ws.ProjectID, discoveryProjectID)
		key := ImplWorkspaceKeyFor(projectID, slug)
		seenKey := implWorkspaceSeenKey(key)
		if slug == "" || ws.Status == StatusInvalid || seen.Has(seenKey) {
			continue
		}
		seen.Add(seenKey)

		gitState := InspectImplWorkspaceGit(ctx, ws.CheckoutPath, input.TrunkBranch)
		before, beforeErr := s.Queries.GetImplWorkspace(ctx, db.GetImplWorkspaceParams{ProjectID: key.ProjectID, WorkspaceSlug: key.Slug})
		if beforeErr != nil && !errors.Is(beforeErr, sql.ErrNoRows) {
			return ImplWorkspaceSyncResult{}, beforeErr
		}
		if beforeErr == nil {
			gitState = applyCachedMergeProof(gitState, before)
		}
		binding := readBestEffortPlanBinding(ws.CheckoutPath)
		params := implWorkspaceUpsertParams(projectID, ws, gitState, binding)
		row, err := s.Queries.UpsertDiscoveredImplWorkspace(ctx, params)
		if err != nil {
			return ImplWorkspaceSyncResult{}, err
		}
		if err := upsertPlanBindingForImplWorkspace(ctx, s.Queries, projectID, ws, binding); err != nil {
			return ImplWorkspaceSyncResult{}, err
		}
		result.Upserted++
		rowChanged := errors.Is(beforeErr, sql.ErrNoRows) ||
			implWorkspaceRowChanged(before, row)
		if rowChanged {
			result.Changed = true
			if row.Status == string(ImplWorkspaceStatusMerged) {
				result.Merged++
			}
		}

		action, repairErr := ReconcileWorkspaceEnv(
			ctx,
			ws,
			input.ManagerURL,
			input.RestartToken,
		)
		switch action {
		case ImplWorkspaceEnvActionCreated, ImplWorkspaceEnvActionRepaired:
			result.RepairedEnv++
			result.Changed = true
			if err := s.Queries.RecordImplWorkspaceEnvRepair(ctx, db.RecordImplWorkspaceEnvRepairParams{ProjectID: key.ProjectID, WorkspaceSlug: key.Slug}); err != nil {
				return ImplWorkspaceSyncResult{}, err
			}
		}
		if repairErr != nil {
			if err := s.Queries.RecordImplWorkspaceEnvError(
				ctx,
				db.RecordImplWorkspaceEnvErrorParams{
					ProjectID:     key.ProjectID,
					WorkspaceSlug: key.Slug,
					EnvLastError:  nullableString(repairErr.Error()),
				},
			); err != nil {
				return ImplWorkspaceSyncResult{}, err
			}
		}
	}

	cleaned, merged, missingChanged, err := s.reconcileMissing(ctx, input, seen)
	if err != nil {
		return ImplWorkspaceSyncResult{}, err
	}
	result.CleanedUp = cleaned
	result.Merged += merged
	if cleaned > 0 || merged > 0 || missingChanged {
		result.Changed = true
	}
	return result, nil
}

func ReconcileWorkspaceEnv(
	ctx context.Context,
	ws Workspace,
	managerURL string,
	restartToken string,
) (ImplWorkspaceEnvAction, error) {
	_ = ctx
	managerURL = strings.TrimSpace(managerURL)
	restartToken = strings.TrimSpace(restartToken)
	if managerURL == "" || restartToken == "" {
		return ImplWorkspaceEnvActionSkipped, nil
	}
	expected := WorkspaceMetadata{
		Slug:         ws.Slug,
		CheckoutPath: ws.CheckoutPath,
		ManagerURL:   managerURL,
		RestartToken: restartToken,
		DatabasePath: RuntimePaths(ws.CheckoutPath, ws.MetadataDirName).AgentsDB,
	}
	metadataPath := WorkspaceMetadataPath(ws.CheckoutPath, ws.MetadataDirName)
	existing, err := ReadMetadata(metadataPath)
	if errors.Is(err, os.ErrNotExist) {
		return ImplWorkspaceEnvActionCreated, WriteMetadata(metadataPath, expected)
	}
	if err != nil {
		return ImplWorkspaceEnvActionSkipped, err
	}
	lifecycle, lifecycleErr := FileBundleStore{}.ReadLifecycle(ws)
	if lifecycleErr == nil && (lifecycle.ObservedState == WorkspaceObservedStarting ||
		lifecycle.ObservedState == WorkspaceObservedStopping) {
		return ImplWorkspaceEnvActionSkipped, nil
	}
	if !workspaceMetadataMatches(existing, expected) {
		expected.PID = existing.PID
		expected.Port = existing.Port
		return ImplWorkspaceEnvActionRepaired, WriteMetadata(metadataPath, expected)
	}
	return ImplWorkspaceEnvActionNone, nil
}

func workspaceMetadataMatches(existing, expected WorkspaceMetadata) bool {
	return strings.TrimSpace(existing.Slug) == strings.TrimSpace(expected.Slug) &&
		cleanPathKey(existing.CheckoutPath) == cleanPathKey(expected.CheckoutPath) &&
		strings.TrimSpace(
			existing.ManagerURL,
		) == strings.TrimSpace(
			expected.ManagerURL,
		) &&
		strings.TrimSpace(
			existing.RestartToken,
		) == strings.TrimSpace(
			expected.RestartToken,
		) &&
		cleanPathKey(existing.DatabasePath) == cleanPathKey(expected.DatabasePath)
}

func readBestEffortPlanBinding(checkoutPath string) PlanWorkspaceBinding {
	binding, err := ReadPlanWorkspaceBinding(PlanWorkspaceBindingPath(checkoutPath))
	if err != nil {
		return PlanWorkspaceBinding{}
	}
	return binding
}

func upsertPlanBindingForImplWorkspace(
	ctx context.Context,
	q *db.Queries,
	projectID string,
	ws Workspace,
	binding PlanWorkspaceBinding,
) error {
	planDir := strings.TrimSpace(binding.PlanDir)
	if planDir == "" {
		return nil
	}
	if _, err := q.GetPlanWorkspace(ctx, planDir); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	bindingProjectID := strings.TrimSpace(firstNonEmpty(binding.ProjectID, projectID))
	if bindingProjectID == "" {
		return nil
	}
	_, err := q.UpsertPlanWorkspaceImplBinding(ctx, db.UpsertPlanWorkspaceImplBindingParams{
		PlanDirRel:        planDir,
		ProjectID:         bindingProjectID,
		WorkspaceSlug:     nullableString(ws.Slug),
		CheckoutPath:      nullableString(ws.CheckoutPath),
		Url:               nullableString(ws.URL),
		Status:            string(ImplWorkspaceStatusActive),
		BindingSource:     "binding_file",
		ImplProjectID:     nullableString(projectID),
		ImplWorkspaceSlug: nullableString(ws.Slug),
	})
	return err
}

func implWorkspaceUpsertParams(
	projectID string,
	ws Workspace,
	gitState ImplWorkspaceGitState,
	binding PlanWorkspaceBinding,
) db.UpsertDiscoveredImplWorkspaceParams {
	status := string(ImplWorkspaceStatusActive)
	mergeEvidence := sql.NullString{}
	proof := gitState.MergeProof
	if proof.Kind == "" {
		proof.Kind = MergeProofUnknown
	}
	protected := IsProtectedCheckoutRole(ws.CheckoutRole) || ws.IsMain || ws.Slug == mainWorkspaceSlug
	if gitState.Merged && !protected {
		status = string(ImplWorkspaceStatusMerged)
		mergeEvidence = nullableString(activeCheckoutMergeEvidence(gitState.Commit, proof))
	}
	params := db.UpsertDiscoveredImplWorkspaceParams{
		ProjectID:                strings.TrimSpace(projectID),
		WorkspaceSlug:            ws.Slug,
		CheckoutRole:             string(ws.CheckoutRole),
		CheckoutPath:             ws.CheckoutPath,
		DisplayName:              workspaceNavLabel(ws),
		Host:                     ws.Host,
		Url:                      ws.URL,
		Status:                   status,
		MergeEvidence:            mergeEvidence,
		CleanupProofKind:         string(proof.Kind),
		CleanupProofSourceRef:    nullableString(proof.SourceRef),
		CleanupProofTargetCommit: nullableString(proof.TargetCommit),
		CleanupRiskReason:        nullableString(proof.RiskReason),
		AheadCount:               int64(gitState.AheadCount),
		BehindCount:              int64(gitState.BehindCount),
	}
	params.Branch = nullableString(firstNonEmpty(gitState.Branch, ws.Branch))
	params.CommitHash = nullableString(firstNonEmpty(gitState.Commit, ws.Commit))
	params.TrunkBranch = nullableString(gitState.TrunkBranch)
	params.TopBranch = nullableString(gitState.TopBranch)
	params.BottomBranch = nullableString(gitState.BottomBranch)
	params.BottomParentBranch = nullableString(gitState.BottomParent)
	params.BaseBranch = nullableString(gitState.BaseBranch)
	params.GitDetail = nullableString(gitState.Detail)
	if !proof.ProvenAt.IsZero() {
		params.CleanupProofAt = sql.NullTime{Time: proof.ProvenAt, Valid: true}
	}
	if strings.TrimSpace(binding.PlanDir) != "" {
		params.PlanDirRel = nullableString(binding.PlanDir)
		params.PlanDir = nullableString(binding.PlanDir)
	}
	return params
}

func activeCheckoutMergeEvidence(commit string, proof MergeProof) string {
	commit = strings.TrimSpace(commit)
	ref := strings.TrimSpace(proof.SourceRef)
	if ref == "" {
		ref = "trunk"
	}
	head := "active checkout HEAD"
	if commit != "" {
		head += " " + commit
	}
	switch proof.Kind {
	case MergeProofPatchEquivalent:
		return head + " is patch-equivalent to " + ref
	case MergeProofCached:
		return head + " uses cached merge proof for " + ref
	default:
		return head + " is ancestor of " + ref
	}
}

func missingCheckoutMergeEvidence(row db.ImplWorkspace, proof MergeProof) string {
	commit := nullStringValue(row.CommitHash)
	ref := firstNonEmpty(proof.SourceRef, "origin/main")
	if proof.Detail != "" {
		return proof.Detail
	}
	if commit == "" {
		return strings.TrimSpace(proof.RiskReason)
	}
	return "missing checkout commit " + commit + " is ancestor of " + ref
}

func applyCachedMergeProof(state ImplWorkspaceGitState, before db.ImplWorkspace) ImplWorkspaceGitState {
	if state.MergeProof.Strong() || !sameStoredHead(before, state.Commit) {
		return state
	}
	kind := MergeProofKind(strings.TrimSpace(before.CleanupProofKind))
	if kind != MergeProofAncestor && kind != MergeProofPatchEquivalent && kind != MergeProofCached {
		return state
	}
	state.Merged = true
	state.MergeRef = nullStringValue(before.CleanupProofSourceRef)
	state.MergeProof = MergeProof{
		Kind:         MergeProofCached,
		SourceRef:    nullStringValue(before.CleanupProofSourceRef),
		TargetCommit: nullStringValue(before.CleanupProofTargetCommit),
		ProvenAt:     time.Now(),
		Detail:       "cached merge proof for unchanged HEAD",
	}
	return state
}

func sameStoredHead(row db.ImplWorkspace, head string) bool {
	return strings.TrimSpace(nullStringValue(row.CommitHash)) != "" && strings.TrimSpace(nullStringValue(row.CommitHash)) == strings.TrimSpace(head)
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *ImplWorkspaceSyncer) reconcileMissing(
	ctx context.Context,
	input ImplWorkspaceSyncInput,
	activeSlugs collections.Set[string],
) (cleaned, merged int, changed bool, err error) {
	rows, err := s.Queries.ListImplWorkspaces(ctx, firstNonEmpty(input.Discovery.ProjectID, input.ProjectID))
	if err != nil {
		return 0, 0, false, err
	}
	configured := configuredCheckoutSlugs(input.Discovery)
	for _, row := range rows {
		rowKey := implWorkspaceSeenKey(ImplWorkspaceKeyFor(row.ProjectID, row.WorkspaceSlug))
		if row.Status != string(ImplWorkspaceStatusActive) ||
			activeSlugs.Has(rowKey) ||
			configured.Has(row.WorkspaceSlug) ||
			IsProtectedCheckoutRole(CheckoutRole(strings.TrimSpace(row.CheckoutRole))) ||
			row.WorkspaceSlug == "stage" ||
			!rowInDiscoveryScope(row, input.Discovery) {
			continue
		}
		status, proof := DetermineMissingWorkspaceStatus(
			ctx,
			input.Discovery.MainCheckoutPath,
			row,
		)
		if status == ImplWorkspaceStatusMerged {
			n, err := s.Queries.MarkImplWorkspaceMerged(
				ctx,
				db.MarkImplWorkspaceMergedParams{
					ProjectID:                row.ProjectID,
					WorkspaceSlug:            row.WorkspaceSlug,
					MergeEvidence:            nullableString(missingCheckoutMergeEvidence(row, proof)),
					CleanupProofKind:         string(proof.Kind),
					CleanupProofSourceRef:    nullableString(proof.SourceRef),
					CleanupProofTargetCommit: nullableString(proof.TargetCommit),
				},
			)
			if err != nil {
				return cleaned, merged, changed, err
			}
			merged += int(n)
			changed = changed || n > 0
			continue
		}
		n, err := s.Queries.MarkImplWorkspaceMergeUnknown(ctx, db.MarkImplWorkspaceMergeUnknownParams{
			ProjectID:             row.ProjectID,
			WorkspaceSlug:         row.WorkspaceSlug,
			CleanupProofSourceRef: nullableString(proof.SourceRef),
			CleanupRiskReason:     nullableString(proof.RiskReason),
			MergeEvidence:         nullableString(proof.RiskReason),
		})
		if err != nil {
			return cleaned, merged, changed, err
		}
		changed = changed || n > 0
	}
	return cleaned, merged, changed, nil
}

func implWorkspaceSeenKey(key ImplWorkspaceKey) string {
	return strings.TrimSpace(key.ProjectID) + "\x00" + strings.TrimSpace(key.Slug)
}

func configuredCheckoutSlugs(cfg ImplWorkspaceDiscoveryConfig) collections.Set[string] {
	slugs := collections.NewSet[string]()
	for slug := range cfg.ConfiguredCheckouts {
		slug = strings.TrimSpace(slug)
		if slug != "" {
			slugs.Add(slug)
		}
	}
	return slugs
}

func rowInDiscoveryScope(row db.ImplWorkspace, cfg ImplWorkspaceDiscoveryConfig) bool {
	discovery := NormalizeDiscoveryConfig(DiscoveryConfig{
		MainCheckoutPath: cfg.MainCheckoutPath,
		ParentDir:        cfg.ParentDir,
		CheckoutPrefixes: cfg.CheckoutPrefixes,
		MainCheckoutName: cfg.MainCheckoutName,
	})
	checkout := strings.TrimSpace(row.CheckoutPath)
	if checkout == "" {
		return false
	}
	if samePath(checkout, discovery.MainCheckoutPath) {
		return true
	}
	parent, err := discoveryParentDir(discovery)
	if err != nil || strings.TrimSpace(parent) == "" {
		return false
	}
	cleanParent := cleanPathKey(parent)
	cleanCheckout := cleanPathKey(checkout)
	if cleanParent == "" || cleanCheckout == "" || strings.TrimPrefix(cleanCheckout, cleanParent) == cleanCheckout {
		return false
	}
	rel, err := filepath.Rel(parent, checkout)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	if strings.Contains(rel, string(filepath.Separator)) {
		return false
	}
	return hasConfiguredPrefix(filepath.Base(checkout), discovery.CheckoutPrefixes)
}

func implWorkspaceRowChanged(before, after db.ImplWorkspace) bool {
	return before.ProjectID != after.ProjectID ||
		before.CheckoutRole != after.CheckoutRole ||
		before.CheckoutPath != after.CheckoutPath ||
		before.DisplayName != after.DisplayName ||
		before.Host != after.Host ||
		before.Url != after.Url ||
		before.Status != after.Status ||
		!nullStringsEqual(before.Branch, after.Branch) ||
		!nullStringsEqual(before.CommitHash, after.CommitHash) ||
		!nullStringsEqual(before.PlanDirRel, after.PlanDirRel) ||
		!nullStringsEqual(before.PlanDir, after.PlanDir) ||
		!nullStringsEqual(before.TrunkBranch, after.TrunkBranch) ||
		!nullStringsEqual(before.TopBranch, after.TopBranch) ||
		!nullStringsEqual(before.BottomBranch, after.BottomBranch) ||
		!nullStringsEqual(before.BottomParentBranch, after.BottomParentBranch) ||
		!nullStringsEqual(before.BaseBranch, after.BaseBranch) ||
		before.AheadCount != after.AheadCount ||
		before.BehindCount != after.BehindCount ||
		!nullStringsEqual(before.MergeEvidence, after.MergeEvidence) ||
		before.CleanupProofKind != after.CleanupProofKind ||
		!nullStringsEqual(before.CleanupProofSourceRef, after.CleanupProofSourceRef) ||
		!nullStringsEqual(before.CleanupProofTargetCommit, after.CleanupProofTargetCommit) ||
		!nullStringsEqual(before.CleanupRiskReason, after.CleanupRiskReason) ||
		!nullStringsEqual(before.GitDetail, after.GitDetail) ||
		!nullTimesEqual(before.MergedAt, after.MergedAt)
}

func nullTimesEqual(a, b sql.NullTime) bool {
	if a.Valid != b.Valid {
		return false
	}
	if !a.Valid {
		return true
	}
	return a.Time.Equal(b.Time)
}

func nullStringsEqual(a, b sql.NullString) bool {
	if a.Valid != b.Valid {
		return false
	}
	if !a.Valid {
		return true
	}
	return a.String == b.String
}
