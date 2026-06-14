package workspaces

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

type WorkspaceDiagnosticSource string

const (
	WorkspaceDiagnosticSourceSync WorkspaceDiagnosticSource = "scheduled_sync_diagnostics"
)

type WorkspaceDiagnosticSeverity string

const (
	WorkspaceDiagnosticWarning WorkspaceDiagnosticSeverity = "warning"
)

type WorkspaceDiagnostic struct {
	Source        WorkspaceDiagnosticSource   `json:"source"`
	Severity      WorkspaceDiagnosticSeverity `json:"severity"`
	Code          string                      `json:"code"`
	Message       string                      `json:"message"`
	Detail        string                      `json:"detail,omitempty"`
	ProjectID     string                      `json:"project_id,omitempty"`
	WorkspaceSlug string                      `json:"workspace_slug,omitempty"`
	CheckoutPath  string                      `json:"checkout_path,omitempty"`
}

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
	Scanned     int                   `json:"scanned"`
	Discovered  int                   `json:"discovered"`
	Upserted    int                   `json:"upserted"`
	RepairedEnv int                   `json:"repaired_env"`
	CleanedUp   int                   `json:"cleaned_up"`
	Merged      int                   `json:"merged"`
	Changed     bool                  `json:"changed"`
	Warnings    []WorkspaceDiagnostic `json:"warnings,omitempty"`
}

func (s *ImplWorkspaceSyncer) Sync(
	ctx context.Context,
	input ImplWorkspaceSyncInput,
) (result ImplWorkspaceSyncResult, err error) {
	if s == nil || s.Queries == nil {
		return ImplWorkspaceSyncResult{}, errors.New(
			"impl workspace syncer requires queries",
		)
	}
	started := s.now()
	defer func() {
		recordErr := s.recordSyncDiagnostic(ctx, input, started, result, err)
		if recordErr == nil {
			return
		}
		if err != nil {
			err = fmt.Errorf("%w; additionally failed to record workspace sync diagnostic: %v", err, recordErr)
			return
		}
		err = recordErr
	}()

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

	if _, err := s.Queries.ClearInvalidImplWorkspacePlanRefs(ctx); err != nil {
		return ImplWorkspaceSyncResult{}, err
	}

	result = ImplWorkspaceSyncResult{
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
		binding, err := resolvePlanWorkspaceBinding(
			ctx,
			s.Queries,
			readBestEffortPlanBinding(ws.CheckoutPath),
		)
		if err != nil {
			return ImplWorkspaceSyncResult{}, err
		}
		params := implWorkspaceUpsertParams(projectID, ws, gitState, binding)
		if _, err := s.Queries.ReassignImplWorkspaceCheckoutPathIdentity(ctx, db.ReassignImplWorkspaceCheckoutPathIdentityParams{
			ProjectID:     params.ProjectID,
			WorkspaceSlug: params.WorkspaceSlug,
			CheckoutPath:  params.CheckoutPath,
		}); err != nil {
			return ImplWorkspaceSyncResult{}, err
		}
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
			result.Warnings = append(result.Warnings, WorkspaceDiagnostic{
				Source:        WorkspaceDiagnosticSourceSync,
				Severity:      WorkspaceDiagnosticWarning,
				Code:          "workspace_env_repair_failed",
				Message:       "Scheduled sync could not repair workspace.env.",
				Detail:        repairErr.Error(),
				ProjectID:     key.ProjectID,
				WorkspaceSlug: key.Slug,
				CheckoutPath:  ws.CheckoutPath,
			})
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

	cleaned, merged, missingChanged, warnings, err := s.reconcileMissing(ctx, input, seen)
	if err != nil {
		return ImplWorkspaceSyncResult{}, err
	}
	result.CleanedUp = cleaned
	result.Merged += merged
	result.Warnings = append(result.Warnings, warnings...)
	if cleaned > 0 || merged > 0 || missingChanged {
		result.Changed = true
	}
	return result, nil
}

func (s *ImplWorkspaceSyncer) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *ImplWorkspaceSyncer) recordSyncDiagnostic(ctx context.Context, input ImplWorkspaceSyncInput, started time.Time, result ImplWorkspaceSyncResult, syncErr error) error {
	finished := s.now()
	status := "ok"
	errorText := ""
	if syncErr != nil {
		status = "error"
		errorText = syncErr.Error()
	}
	warnings := result.Warnings
	if warnings == nil {
		warnings = []WorkspaceDiagnostic{}
	}
	warningsJSON, err := json.Marshal(warnings)
	if err != nil {
		return err
	}
	projectID := firstNonEmpty(input.Discovery.ProjectID, input.ProjectID)
	return s.Queries.UpsertWorkspaceSyncDiagnostic(ctx, db.UpsertWorkspaceSyncDiagnosticParams{
		ProjectID:    projectID,
		SyncKind:     "impl_workspaces",
		StartedAt:    started,
		FinishedAt:   sql.NullTime{Time: finished, Valid: true},
		Status:       status,
		Error:        errorText,
		Scanned:      int64(result.Scanned),
		Discovered:   int64(result.Discovered),
		Upserted:     int64(result.Upserted),
		RepairedEnv:  int64(result.RepairedEnv),
		Merged:       int64(result.Merged),
		CleanedUp:    int64(result.CleanedUp),
		Changed:      result.Changed,
		WarningsJson: string(warningsJSON),
	})
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
		ProjectID:    ws.ProjectID,
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
		strings.TrimSpace(existing.ProjectID) == strings.TrimSpace(expected.ProjectID) &&
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

func resolvePlanWorkspaceBinding(
	ctx context.Context,
	q *db.Queries,
	binding PlanWorkspaceBinding,
) (PlanWorkspaceBinding, error) {
	planDirRel, err := canonicalPlanDirRelForBinding(ctx, q, binding.PlanDir)
	if err != nil {
		return PlanWorkspaceBinding{}, err
	}
	binding.PlanDir = planDirRel
	return binding, nil
}

func canonicalPlanDirRelForBinding(
	ctx context.Context,
	q *db.Queries,
	planDir string,
) (string, error) {
	for _, candidate := range planDirRelCandidates(planDir) {
		row, err := q.GetPlanWorkspace(ctx, candidate)
		if err == nil {
			return row.PlanDirRel, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	return "", nil
}

func planDirRelCandidates(planDir string) []string {
	candidate := normalizePlanDir(planDir)
	if candidate == "" {
		return nil
	}
	candidates := []string{candidate}
	if strings.HasPrefix(candidate, "thoughts/") {
		candidates = append(candidates, strings.TrimPrefix(candidate, "thoughts/"))
	}
	if idx := strings.LastIndex(candidate, "/thoughts/"); idx >= 0 {
		candidates = append(candidates, candidate[idx+len("/thoughts/"):])
	}
	out := make([]string, 0, len(candidates)*2)
	seen := collections.NewSet[string]()
	for _, value := range candidates {
		for _, normalized := range []string{value, stripPlanArtifactFilename(value)} {
			normalized = normalizePlanDir(normalized)
			if normalized == "" || seen.Has(normalized) {
				continue
			}
			seen.Add(normalized)
			out = append(out, normalized)
		}
	}
	return out
}

func stripPlanArtifactFilename(planDir string) string {
	switch filepath.Base(planDir) {
	case "plan.md", "outline.md", "design.md", "design-product.md", "verify.md", "review.md":
		return filepath.Dir(planDir)
	default:
		return planDir
	}
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
	ws.ProjectID = firstNonEmpty(ws.ProjectID, projectID)
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
	params.ActivityHash = workspaceActivityHash(ws, gitState, binding)
	if !proof.ProvenAt.IsZero() {
		params.CleanupProofAt = sql.NullTime{Time: proof.ProvenAt, Valid: true}
	}
	if strings.TrimSpace(binding.PlanDir) != "" {
		params.PlanDirRel = nullableString(binding.PlanDir)
		params.PlanDir = nullableString(binding.PlanDir)
	}
	return params
}

func workspaceActivityHash(
	ws Workspace,
	gitState ImplWorkspaceGitState,
	binding PlanWorkspaceBinding,
) string {
	parts := []string{
		"project=" + strings.TrimSpace(ws.ProjectID),
		"slug=" + strings.TrimSpace(ws.Slug),
		"checkout_role=" + strings.TrimSpace(string(ws.CheckoutRole)),
		"checkout_path=" + cleanPathKey(ws.CheckoutPath),
		"branch=" + strings.TrimSpace(firstNonEmpty(gitState.Branch, ws.Branch)),
		"commit=" + strings.TrimSpace(firstNonEmpty(gitState.Commit, ws.Commit)),
		"trunk=" + strings.TrimSpace(gitState.TrunkBranch),
		"top=" + strings.TrimSpace(gitState.TopBranch),
		"bottom=" + strings.TrimSpace(gitState.BottomBranch),
		"bottom_parent=" + strings.TrimSpace(gitState.BottomParent),
		"base=" + strings.TrimSpace(gitState.BaseBranch),
		fmt.Sprintf("ahead=%d", gitState.AheadCount),
		fmt.Sprintf("behind=%d", gitState.BehindCount),
		"git_detail=" + strings.TrimSpace(gitState.Detail),
		"plan_dir=" + strings.TrimSpace(binding.PlanDir),
		"binding_project=" + strings.TrimSpace(binding.ProjectID),
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
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
) (cleaned, merged int, changed bool, warnings []WorkspaceDiagnostic, err error) {
	rows, err := s.Queries.ListImplWorkspaces(ctx, firstNonEmpty(input.Discovery.ProjectID, input.ProjectID))
	if err != nil {
		return 0, 0, false, nil, err
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
				return cleaned, merged, changed, warnings, err
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
			return cleaned, merged, changed, warnings, err
		}
		if n > 0 {
			warnings = append(warnings, WorkspaceDiagnostic{
				Source:        WorkspaceDiagnosticSourceSync,
				Severity:      WorkspaceDiagnosticWarning,
				Code:          "merge_proof_unknown",
				Message:       "Scheduled sync could not prove this missing checkout is merged.",
				Detail:        firstNonEmpty(proof.RiskReason, proof.Detail, "merge proof unavailable"),
				ProjectID:     row.ProjectID,
				WorkspaceSlug: row.WorkspaceSlug,
				CheckoutPath:  row.CheckoutPath,
			})
		}
		changed = changed || n > 0
	}
	return cleaned, merged, changed, warnings, nil
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
		before.ActivityHash != after.ActivityHash ||
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
