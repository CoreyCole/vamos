package workspaces

import (
	"context"
	"database/sql"
	"errors"
	"os"
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
	discovered, err := Discover(DiscoveryConfig{
		MainCheckoutPath: input.Discovery.MainCheckoutPath,
		ParentDir:        input.Discovery.ParentDir,
		Domain:           input.Discovery.Domain,
		MetadataDirName:  input.Discovery.MetadataDirName,
		CheckoutPrefixes: input.Discovery.CheckoutPrefixes,
		MainCheckoutName: input.Discovery.MainCheckoutName,
		ModuleMarker:     input.Discovery.ModuleMarker,
		PackageSubdir:    input.Discovery.PackageSubdir,
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
		if slug == "" || ws.Status == StatusInvalid || seen.Has(slug) {
			continue
		}
		seen.Add(slug)

		gitState := InspectImplWorkspaceGit(ctx, ws.CheckoutPath, input.TrunkBranch)
		binding := readBestEffortPlanBinding(ws.CheckoutPath)
		params := implWorkspaceUpsertParams(ws, gitState, binding)
		before, beforeErr := s.Queries.GetImplWorkspace(ctx, slug)
		row, err := s.Queries.UpsertDiscoveredImplWorkspace(ctx, params)
		if err != nil {
			return ImplWorkspaceSyncResult{}, err
		}
		result.Upserted++
		if beforeErr != nil && !errors.Is(beforeErr, sql.ErrNoRows) {
			return ImplWorkspaceSyncResult{}, beforeErr
		}
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
			if err := s.Queries.RecordImplWorkspaceEnvRepair(ctx, slug); err != nil {
				return ImplWorkspaceSyncResult{}, err
			}
		}
		if repairErr != nil {
			if err := s.Queries.RecordImplWorkspaceEnvError(
				ctx,
				db.RecordImplWorkspaceEnvErrorParams{
					WorkspaceSlug: slug,
					EnvLastError:  nullableString(repairErr.Error()),
				},
			); err != nil {
				return ImplWorkspaceSyncResult{}, err
			}
		}
	}

	cleaned, merged, err := s.reconcileMissing(ctx, input, seen)
	if err != nil {
		return ImplWorkspaceSyncResult{}, err
	}
	result.CleanedUp = cleaned
	result.Merged += merged
	if cleaned > 0 || merged > 0 {
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
		)
}

func readBestEffortPlanBinding(checkoutPath string) PlanWorkspaceBinding {
	binding, err := ReadPlanWorkspaceBinding(PlanWorkspaceBindingPath(checkoutPath))
	if err != nil {
		return PlanWorkspaceBinding{}
	}
	return binding
}

func implWorkspaceUpsertParams(
	ws Workspace,
	gitState ImplWorkspaceGitState,
	binding PlanWorkspaceBinding,
) db.UpsertDiscoveredImplWorkspaceParams {
	status := string(ImplWorkspaceStatusActive)
	mergeEvidence := sql.NullString{}
	if gitState.Merged {
		status = string(ImplWorkspaceStatusMerged)
		mergeEvidence = nullableString(
			activeCheckoutMergeEvidence(gitState.Commit, gitState.MergeRef),
		)
	}
	params := db.UpsertDiscoveredImplWorkspaceParams{
		WorkspaceSlug: ws.Slug,
		CheckoutPath:  ws.CheckoutPath,
		DisplayName:   workspaceNavLabel(ws),
		Host:          ws.Host,
		Url:           ws.URL,
		Status:        status,
		MergeEvidence: mergeEvidence,
		AheadCount:    int64(gitState.AheadCount),
		BehindCount:   int64(gitState.BehindCount),
	}
	params.Branch = nullableString(firstNonEmpty(gitState.Branch, ws.Branch))
	params.CommitHash = nullableString(firstNonEmpty(gitState.Commit, ws.Commit))
	params.TrunkBranch = nullableString(gitState.TrunkBranch)
	params.TopBranch = nullableString(gitState.TopBranch)
	params.BottomBranch = nullableString(gitState.BottomBranch)
	params.BottomParentBranch = nullableString(gitState.BottomParent)
	params.BaseBranch = nullableString(gitState.BaseBranch)
	params.GitDetail = nullableString(gitState.Detail)
	if strings.TrimSpace(binding.PlanDir) != "" {
		params.PlanDirRel = nullableString(binding.PlanDir)
		params.PlanDir = nullableString(binding.PlanDir)
	}
	return params
}

func activeCheckoutMergeEvidence(commit, ref string) string {
	commit = strings.TrimSpace(commit)
	ref = strings.TrimSpace(ref)
	if commit == "" && ref == "" {
		return "active checkout HEAD is ancestor of trunk"
	}
	if commit == "" {
		return "active checkout HEAD is ancestor of " + ref
	}
	if ref == "" {
		return "active checkout HEAD " + commit + " is ancestor of trunk"
	}
	return "active checkout HEAD " + commit + " is ancestor of " + ref
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
) (cleaned, merged int, err error) {
	rows, err := s.Queries.ListImplWorkspaces(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, row := range rows {
		if row.Status != string(ImplWorkspaceStatusActive) ||
			activeSlugs.Has(row.WorkspaceSlug) {
			continue
		}
		status, evidence := DetermineMissingWorkspaceStatus(
			ctx,
			input.Discovery.MainCheckoutPath,
			row,
		)
		if status == ImplWorkspaceStatusMerged {
			n, err := s.Queries.MarkImplWorkspaceMerged(
				ctx,
				db.MarkImplWorkspaceMergedParams{
					WorkspaceSlug: row.WorkspaceSlug,
					MergeEvidence: nullableString(evidence),
				},
			)
			if err != nil {
				return cleaned, merged, err
			}
			merged += int(n)
			continue
		}
		n, err := s.Queries.MarkImplWorkspaceCleanedUp(ctx, row.WorkspaceSlug)
		if err != nil {
			return cleaned, merged, err
		}
		cleaned += int(n)
	}
	return cleaned, merged, nil
}

func implWorkspaceRowChanged(before, after db.ImplWorkspace) bool {
	return before.CheckoutPath != after.CheckoutPath ||
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
