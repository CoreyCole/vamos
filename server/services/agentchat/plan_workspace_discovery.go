package agentchat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/activity"

	"github.com/CoreyCole/vamos/pkg/collections"
	"github.com/CoreyCole/vamos/pkg/db"
	"github.com/CoreyCole/vamos/server/services/planworkspace"
	"github.com/CoreyCole/vamos/server/services/workspaces"
)

const (
	defaultPlanWorkspaceDiscoveryInterval = time.Minute
	planWorkspaceRootPartCount            = 3
	planWorkspaceResearchDir              = "research"
)

type PlanWorkspaceDiscoveryInput struct {
	ProjectName        string
	ProjectID          string
	ProjectInstanceKey string
	ProjectRoot        string
	ThoughtsRoot       string
	ImplWorkspaces     workspaces.ImplWorkspaceDiscoveryConfig
}

type DiscoveredPlanWorkspace struct {
	ProjectID                 string
	PlanDir                   string
	PlanDirRel                string
	Label                     string
	WorkspaceSlug             string
	ImplWorkspacePath         string
	ImplWorkspaceURL          string
	ImplWorkspaceDiscoveredAt time.Time
	ArtifactUpdatedAt         time.Time
	QRSPIStage                planworkspace.QRSPIStage
	QRSPILifecycleUpdatedAt   time.Time
	QRSPIClosedReason         string
}

type PlanWorkspaceDiscoveryResult struct {
	Scanned              int
	Discovered           int
	Upserted             int
	Archived             int
	Restored             int
	AgentSessionsIndexed int
	Changed              bool
	MaxArtifactUpdatedAt time.Time
}

type WorkspaceDocKind string

const (
	WorkspaceDocKindFile WorkspaceDocKind = "file"
	WorkspaceDocKindDir  WorkspaceDocKind = "dir"
)

type WorkspaceDocAction string

const (
	WorkspaceDocActionUpsert WorkspaceDocAction = "upsert"
	WorkspaceDocActionDelete WorkspaceDocAction = "delete"
)

type WorkspaceDocChange struct {
	RelPath string
	DocPath string
	Kind    WorkspaceDocKind
	Action  WorkspaceDocAction
}

type PlanWorkspaceNotifier interface {
	NotifyProjectPlanSidebar() WorkspaceStreamSignal
}

type PlanWorkspaceScanner struct {
	ProjectID      string
	ThoughtsRoot   string
	ImplWorkspaces workspaces.ImplWorkspaceDiscoveryConfig
	Now            func() time.Time
}

type PlanWorkspaceSyncer struct {
	Queries  *db.Queries
	Scanner  PlanWorkspaceScanner
	Notifier PlanWorkspaceNotifier
}

func (s PlanWorkspaceScanner) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func ExpectedImplWorkspacePath(
	planDir string,
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
) string {
	cfg = normalizeImplWorkspaceDiscoveryConfig(cfg)
	parent := strings.TrimSpace(cfg.ParentDir)
	if parent == "" && strings.TrimSpace(cfg.MainCheckoutPath) != "" {
		parent = filepath.Dir(filepath.Clean(cfg.MainCheckoutPath))
	}
	if parent == "" {
		return ""
	}
	prefix := firstCheckoutPrefix(cfg.CheckoutPrefixes)
	return filepath.Join(parent, prefix+"-"+filepath.Base(filepath.Clean(planDir)))
}

func DiscoverImplWorkspace(
	planDir string,
	slug string,
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
	discoveredAt time.Time,
) (path, url string, at time.Time, ok bool) {
	expected := ExpectedImplWorkspacePath(planDir, cfg)
	if isValidImplCheckout(expected, cfg) {
		return filepath.Clean(
				expected,
			), workspaces.WorkspaceURL(
				slug,
				cfg.Domain,
			), discoveredAt, true
	}
	return discoverBoundOrSyncedImplWorkspace(planDir, cfg, discoveredAt)
}

func discoverBoundOrSyncedImplWorkspace(
	planDir string,
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
	discoveredAt time.Time,
) (path, url string, at time.Time, ok bool) {
	parent := strings.TrimSpace(cfg.ParentDir)
	if parent == "" && strings.TrimSpace(cfg.MainCheckoutPath) != "" {
		parent = filepath.Dir(filepath.Clean(cfg.MainCheckoutPath))
	}
	if parent == "" {
		return "", "", time.Time{}, false
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", "", time.Time{}, false
	}
	var syncedFallback string
	var syncedFallbackSlug string
	cfg = normalizeImplWorkspaceDiscoveryConfig(cfg)
	for _, entry := range entries {
		if !entry.IsDir() ||
			!hasConfiguredCheckoutPrefix(entry.Name(), cfg.CheckoutPrefixes) {
			continue
		}
		checkoutPath := filepath.Join(parent, entry.Name())
		if !isValidImplCheckout(checkoutPath, cfg) {
			continue
		}
		checkoutSlug, err := workspaces.SlugFromCheckoutNameWithConfig(
			entry.Name(),
			implDiscoveryAsDiscoveryConfig(cfg),
		)
		if err != nil {
			continue
		}
		binding, err := workspaces.ReadPlanWorkspaceBinding(
			workspaces.PlanWorkspaceBindingPath(checkoutPath),
		)
		if err == nil && workspaces.PlanWorkspaceBindingMatches(binding, planDir) {
			return filepath.Clean(
					checkoutPath,
				), workspaces.WorkspaceURL(
					checkoutSlug,
					cfg.Domain,
				), discoveredAt, true
		}
		if syncedFallback == "" && syncedPlanDirExists(checkoutPath, planDir) {
			syncedFallback = checkoutPath
			syncedFallbackSlug = checkoutSlug
		}
	}
	if syncedFallback != "" {
		return filepath.Clean(
				syncedFallback,
			), workspaces.WorkspaceURL(
				syncedFallbackSlug,
				cfg.Domain,
			), discoveredAt, true
	}
	return "", "", time.Time{}, false
}

func isValidImplCheckout(
	checkoutPath string,
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
) bool {
	if strings.TrimSpace(checkoutPath) == "" {
		return false
	}
	return workspaces.IsValidCheckout(checkoutPath, implDiscoveryAsDiscoveryConfig(cfg))
}

func normalizeImplWorkspaceDiscoveryConfig(
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
) workspaces.ImplWorkspaceDiscoveryConfig {
	discovery := workspaces.NormalizeDiscoveryConfig(implDiscoveryAsDiscoveryConfig(cfg))
	return workspaces.ImplWorkspaceDiscoveryConfig{
		MainCheckoutPath:    discovery.MainCheckoutPath,
		ParentDir:           discovery.ParentDir,
		Domain:              discovery.Domain,
		MetadataDirName:     discovery.MetadataDirName,
		CheckoutPrefixes:    discovery.CheckoutPrefixes,
		MainCheckoutName:    discovery.MainCheckoutName,
		ModuleMarker:        discovery.ModuleMarker,
		PackageSubdir:       discovery.PackageSubdir,
		ConfiguredCheckouts: discovery.ConfiguredCheckouts,
	}
}

func implDiscoveryAsDiscoveryConfig(
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
) workspaces.DiscoveryConfig {
	return workspaces.DiscoveryConfig{
		MainCheckoutPath:    cfg.MainCheckoutPath,
		ParentDir:           cfg.ParentDir,
		Domain:              cfg.Domain,
		MetadataDirName:     cfg.MetadataDirName,
		CheckoutPrefixes:    cfg.CheckoutPrefixes,
		MainCheckoutName:    cfg.MainCheckoutName,
		ModuleMarker:        cfg.ModuleMarker,
		PackageSubdir:       cfg.PackageSubdir,
		ConfiguredCheckouts: cfg.ConfiguredCheckouts,
	}
}

func firstCheckoutPrefix(prefixes []string) string {
	for _, prefix := range prefixes {
		if prefix = strings.TrimSpace(prefix); prefix != "" {
			return prefix
		}
	}
	return "vamos"
}

func hasConfiguredCheckoutPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" && (name == prefix || strings.HasPrefix(name, prefix+"-")) {
			return true
		}
	}
	return false
}

func syncedPlanDirExists(checkoutPath, planDir string) bool {
	planDir = strings.Trim(strings.TrimSpace(filepath.ToSlash(planDir)), "/")
	if planDir == "" || strings.HasPrefix(planDir, "..") || filepath.IsAbs(planDir) {
		return false
	}
	info, err := os.Stat(filepath.Join(checkoutPath, filepath.FromSlash(planDir)))
	return err == nil && info.IsDir()
}

func (s PlanWorkspaceScanner) Scan(
	ctx context.Context,
) ([]DiscoveredPlanWorkspace, error) {
	thoughtsRoot := filepath.Clean(strings.TrimSpace(s.ThoughtsRoot))
	if thoughtsRoot == "" || thoughtsRoot == "." {
		return nil, errors.New("thoughts root is required")
	}
	if abs, err := filepath.Abs(thoughtsRoot); err == nil {
		thoughtsRoot = abs
	}

	seen := collections.NewSet[string]()
	discovered := []DiscoveredPlanWorkspace{}
	walkCount := 0
	err := filepath.WalkDir(
		thoughtsRoot,
		func(path string, entry fs.DirEntry, walkErr error) error {
			walkCount++
			if walkCount%100 == 0 {
				heartbeatPlanWorkspaceScan(ctx, map[string]any{
					"phase": "discover",
					"path":  path,
					"count": walkCount,
				})
			}
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() {
				return nil
			}
			if shouldSkipPlanWorkspaceDiscoveryDir(thoughtsRoot, path, entry.Name()) {
				return filepath.SkipDir
			}
			if !isPlanWorkspaceRoot(thoughtsRoot, path) {
				return nil
			}

			rel, err := planWorkspaceRel(thoughtsRoot, path)
			if err != nil {
				return err
			}
			if seen.Has(rel) {
				return nil
			}
			seen.Add(rel)

			updatedAt, err := planWorkspaceActivityTimestamp(ctx, path)
			if err != nil {
				return err
			}
			frontmatter, err := readPlanWorkspaceFrontmatter(path)
			if err != nil {
				return err
			}
			slug, err := workspaces.NormalizeWorkspaceSlug(rel)
			if err != nil {
				return fmt.Errorf("derive workspace slug for %s: %w", path, err)
			}
			discoveredAt := s.now()
			implPath, implURL, implAt, implOK := DiscoverImplWorkspace(
				path,
				slug,
				s.ImplWorkspaces,
				discoveredAt,
			)
			item := DiscoveredPlanWorkspace{
				ProjectID:               firstNonEmpty(frontmatter.Project, s.ProjectID),
				PlanDir:                 filepath.Clean(path),
				PlanDirRel:              rel,
				Label:                   planWorkspaceLabel(path),
				WorkspaceSlug:           slug,
				ArtifactUpdatedAt:       updatedAt,
				QRSPIStage:              frontmatter.QRSPIStage,
				QRSPILifecycleUpdatedAt: frontmatter.QRSPILifecycleUpdatedAt,
				QRSPIClosedReason:       frontmatter.QRSPIClosedReason,
			}
			if implOK {
				item.ImplWorkspacePath = implPath
				item.ImplWorkspaceURL = implURL
				item.ImplWorkspaceDiscoveredAt = implAt
			}
			discovered = append(discovered, item)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	sort.Slice(discovered, func(i, j int) bool {
		return discovered[i].PlanDirRel < discovered[j].PlanDirRel
	})
	return discovered, nil
}

func (s *PlanWorkspaceSyncer) Sync(
	ctx context.Context,
	input PlanWorkspaceDiscoveryInput,
) (PlanWorkspaceDiscoveryResult, error) {
	if s == nil || s.Queries == nil {
		return PlanWorkspaceDiscoveryResult{}, errors.New(
			"plan workspace syncer requires queries",
		)
	}
	if strings.TrimSpace(s.Scanner.ThoughtsRoot) == "" {
		s.Scanner.ThoughtsRoot = input.ThoughtsRoot
	}
	if strings.TrimSpace(s.Scanner.ProjectID) == "" {
		s.Scanner.ProjectID = input.ProjectID
	}
	if implWorkspaceDiscoveryConfigIsZero(s.Scanner.ImplWorkspaces) {
		s.Scanner.ImplWorkspaces = input.ImplWorkspaces
	}

	discovered, err := s.Scanner.Scan(ctx)
	if err != nil {
		return PlanWorkspaceDiscoveryResult{}, err
	}

	result := PlanWorkspaceDiscoveryResult{
		Scanned:    len(discovered),
		Discovered: len(discovered),
	}
	rels := make([]string, 0, len(discovered))
	for _, item := range discovered {
		rels = append(rels, item.PlanDirRel)
		before, beforeErr := s.Queries.GetPlanWorkspace(ctx, item.PlanDirRel)
		params := db.UpsertDiscoveredPlanWorkspaceParams{
			PlanDirRel:        item.PlanDirRel,
			ProjectID:         strings.TrimSpace(item.ProjectID),
			PlanDir:           item.PlanDir,
			Label:             item.Label,
			WorkspaceSlug:     item.WorkspaceSlug,
			ArtifactUpdatedAt: item.ArtifactUpdatedAt,
			QrspiLifecycle:    string(item.QRSPIStage),
			QrspiClosedReason: item.QRSPIClosedReason,
		}
		if strings.TrimSpace(item.ImplWorkspacePath) != "" {
			params.ImplWorkspacePath = sql.NullString{
				String: item.ImplWorkspacePath,
				Valid:  true,
			}
		}
		if strings.TrimSpace(item.ImplWorkspaceURL) != "" {
			params.ImplWorkspaceUrl = sql.NullString{
				String: item.ImplWorkspaceURL,
				Valid:  true,
			}
		}
		if !item.ImplWorkspaceDiscoveredAt.IsZero() {
			params.ImplWorkspaceDiscoveredAt = sql.NullTime{
				Time:  item.ImplWorkspaceDiscoveredAt,
				Valid: true,
			}
		}
		if !item.QRSPILifecycleUpdatedAt.IsZero() {
			params.QrspiLifecycleUpdatedAt = sql.NullTime{
				Time:  item.QRSPILifecycleUpdatedAt,
				Valid: true,
			}
		}
		row, err := s.Queries.UpsertDiscoveredPlanWorkspace(ctx, params)
		if err != nil {
			return PlanWorkspaceDiscoveryResult{}, err
		}

		result.Upserted++
		if item.ArtifactUpdatedAt.After(result.MaxArtifactUpdatedAt) {
			result.MaxArtifactUpdatedAt = item.ArtifactUpdatedAt
		}
		indexedSessions, sessionIndexChanged, err := s.syncPlanAgentSessions(ctx, item.PlanDir)
		if err != nil {
			return PlanWorkspaceDiscoveryResult{}, err
		}
		if indexedSessions > 0 {
			result.AgentSessionsIndexed += indexedSessions
		}
		result.Changed = result.Changed || sessionIndexChanged
		if errors.Is(beforeErr, sql.ErrNoRows) {
			result.Changed = true
			continue
		}
		if beforeErr != nil {
			return PlanWorkspaceDiscoveryResult{}, beforeErr
		}
		if before.ArchivedAt.Valid && !row.ArchivedAt.Valid {
			result.Restored++
			result.Changed = true
		}
		if before.ProjectID != row.ProjectID || before.PlanDir != row.PlanDir || before.Label != row.Label ||
			before.WorkspaceSlug != row.WorkspaceSlug ||
			before.QrspiLifecycle != row.QrspiLifecycle ||
			before.QrspiClosedReason != row.QrspiClosedReason ||
			!nullStringsEqual(before.ImplWorkspacePath, row.ImplWorkspacePath) ||
			!nullStringsEqual(before.ImplWorkspaceUrl, row.ImplWorkspaceUrl) ||
			!nullTimesEqual(
				before.ImplWorkspaceDiscoveredAt,
				row.ImplWorkspaceDiscoveredAt,
			) ||
			!nullTimesEqual(
				before.QrspiLifecycleUpdatedAt,
				row.QrspiLifecycleUpdatedAt,
			) ||
			!before.ArtifactUpdatedAt.Equal(row.ArtifactUpdatedAt) {
			result.Changed = true
		}
	}

	var archived int64
	if len(rels) == 0 {
		archived, err = s.Queries.ArchiveAllActivePlanWorkspaces(ctx)
	} else {
		archived, err = s.Queries.ArchiveMissingPlanWorkspaces(ctx, rels)
	}
	if err != nil {
		return PlanWorkspaceDiscoveryResult{}, err
	}
	result.Archived = int(archived)
	if archived > 0 {
		result.Changed = true
	}
	if result.Changed && s.Notifier != nil {
		s.Notifier.NotifyProjectPlanSidebar()
	}
	return result, nil
}

func (s *PlanWorkspaceSyncer) syncPlanAgentSessions(
	ctx context.Context,
	planDir string,
) (int, bool, error) {
	items, err := DiscoverPlanAgentSessionsUnderThoughts(strings.TrimSpace(s.Scanner.ThoughtsRoot), planDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	changed := false
	for _, item := range items {
		before, beforeErr := s.Queries.GetAgentSessionByPath(ctx, nullableString(item.Path))
		metadata, _ := jsonMarshalString(item)
		row, err := s.Queries.UpsertAgentSessionIndex(ctx, db.UpsertAgentSessionIndexParams{
			ID:                     uuid.NewString(),
			IdentityKind:           "plan_owned",
			ArtifactPath:           nullableString(item.Path),
			PlanDir:                nullableString(item.PlanDir),
			ParentPlanDir:          nullableString(item.ParentPlanDir),
			SourceReviewDir:        nullableString(item.SourceReviewDir),
			Agent:                  item.Agent,
			ExternalSessionID:      nullableString(item.SessionID),
			ParentSessionID:        nullableString(item.ContinuedFromSessionID),
			Cwd:                    nullableString(item.CWD),
			WorkflowID:             nullableString(item.WorkflowID),
			WorkflowNodeID:         nullableString(item.NodeID),
			ContinuedFromSessionID: nullableString(item.ContinuedFromSessionID),
			ForkedFromSessionID:    nullableString(item.ForkedFromSessionID),
			FileSize:               item.Size,
			FileMtime:              sql.NullTime{Time: item.MTime, Valid: !item.MTime.IsZero()},
			FileHash:               nullableString(item.Hash),
			LastIndexedOffset:      item.LastOffset,
			ProjectionState:        "needs_hydration",
			ProjectedThreadID:      sql.NullString{},
			IndexedByUserEmail:     sql.NullString{},
			AttachedWorkspaceID:    sql.NullString{},
			LastError:              sql.NullString{},
			MetadataJson:           nullableString(metadata),
		})
		if err != nil {
			return 0, false, err
		}
		if errors.Is(beforeErr, sql.ErrNoRows) || beforeErr == nil && agentSessionIndexChanged(before, row) {
			changed = true
		}
		if beforeErr != nil && !errors.Is(beforeErr, sql.ErrNoRows) {
			return 0, false, beforeErr
		}
	}
	return len(items), changed, nil
}

func agentSessionIndexChanged(before, after db.AgentSession) bool {
	return before.ExternalSessionID != after.ExternalSessionID ||
		before.ParentSessionID != after.ParentSessionID ||
		before.Cwd != after.Cwd ||
		before.PlanDir != after.PlanDir ||
		before.Agent != after.Agent ||
		before.ParentPlanDir != after.ParentPlanDir ||
		before.SourceReviewDir != after.SourceReviewDir ||
		before.WorkflowID != after.WorkflowID ||
		before.WorkflowNodeID != after.WorkflowNodeID ||
		before.ContinuedFromSessionID != after.ContinuedFromSessionID ||
		before.ForkedFromSessionID != after.ForkedFromSessionID ||
		before.FileSize != after.FileSize ||
		before.FileMtime != after.FileMtime ||
		before.FileHash != after.FileHash ||
		before.LastIndexedOffset != after.LastIndexedOffset ||
		before.ProjectionState != after.ProjectionState
}

func jsonMarshalString(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func boolInt(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func implWorkspaceDiscoveryConfigIsZero(
	cfg workspaces.ImplWorkspaceDiscoveryConfig,
) bool {
	return strings.TrimSpace(cfg.MainCheckoutPath) == "" &&
		strings.TrimSpace(cfg.ParentDir) == "" &&
		strings.TrimSpace(cfg.Domain) == "" &&
		strings.TrimSpace(cfg.MetadataDirName) == "" &&
		len(cfg.CheckoutPrefixes) == 0 &&
		strings.TrimSpace(cfg.MainCheckoutName) == "" &&
		strings.TrimSpace(cfg.ModuleMarker) == "" &&
		strings.TrimSpace(cfg.PackageSubdir) == "" &&
		len(cfg.ConfiguredCheckouts) == 0
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

func nullTimesEqual(a, b sql.NullTime) bool {
	if a.Valid != b.Valid {
		return false
	}
	if !a.Valid {
		return true
	}
	return a.Time.Equal(b.Time)
}

func planWorkspaceRel(thoughtsRoot, planDir string) (string, error) {
	root := filepath.Clean(thoughtsRoot)
	dir := filepath.Clean(planDir)
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		rel == ".." {
		return "", fmt.Errorf(
			"plan dir %s is outside thoughts root %s",
			planDir,
			thoughtsRoot,
		)
	}
	return filepath.ToSlash(rel), nil
}

func isPlanWorkspaceRoot(thoughtsRoot, dir string) bool {
	rel, err := planWorkspaceRel(thoughtsRoot, dir)
	if err != nil {
		return false
	}
	parts := strings.Split(rel, "/")
	if len(parts) < 3 || parts[0] == "" || parts[1] != "plans" || parts[2] == "" {
		return false
	}
	if len(parts) == 3 {
		return true
	}
	return hasPlanWorkspaceMarkers(dir)
}

func hasPlanWorkspaceMarkers(dir string) bool {
	for _, name := range []string{"AGENTS.md", "plan.md", "design.md", "outline.md"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	for _, name := range []string{"questions", "reviews"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func shouldSkipPlanWorkspaceDiscoveryDir(thoughtsRoot, dir, name string) bool {
	if filepath.Clean(dir) == filepath.Clean(thoughtsRoot) {
		return false
	}
	if shouldSkipPlanWorkspaceDir(name) {
		return true
	}
	return !isAllowedPlanWorkspaceDiscoveryDir(thoughtsRoot, dir)
}

func isAllowedPlanWorkspaceDiscoveryDir(thoughtsRoot, dir string) bool {
	rel, err := planWorkspaceRel(thoughtsRoot, dir)
	if err != nil {
		return false
	}
	parts := strings.Split(rel, "/")
	if len(parts) <= planWorkspaceRootPartCount {
		return true
	}
	parts = parts[planWorkspaceRootPartCount:]
	for len(parts) > 0 {
		if !isPlanWorkspaceChildContainer(parts[0]) {
			return false
		}
		if len(parts) == 1 {
			return true
		}
		parts = parts[2:]
	}
	return true
}

func isPlanWorkspaceChildContainer(name string) bool {
	switch strings.TrimSpace(name) {
	case "reviews", "milestones":
		return true
	default:
		return false
	}
}

func planWorkspaceLabel(planDir string) string {
	label, _ := formatSidebarGroupDisplay(filepath.Base(filepath.Clean(planDir)))
	if strings.TrimSpace(label) == "" {
		return filepath.Base(filepath.Clean(planDir))
	}
	return label
}

func readPlanWorkspaceFrontmatter(planDir string) (planworkspace.PlanWorkspaceFrontmatter, error) {
	agentsPath := filepath.Join(planDir, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return planworkspace.PlanWorkspaceFrontmatter{QRSPIStage: planworkspace.QRSPIStageQuestion}, nil
		}
		return planworkspace.PlanWorkspaceFrontmatter{}, err
	}
	return planworkspace.ParsePlanWorkspaceFrontmatter(agentsPath, data)
}

func planWorkspaceActivityTimestamp(
	ctx context.Context,
	planDir string,
) (time.Time, error) {
	latest := time.Time{}
	walkCount := 0
	err := filepath.WalkDir(
		planDir,
		func(path string, entry fs.DirEntry, walkErr error) error {
			walkCount++
			if walkCount%100 == 0 {
				heartbeatPlanWorkspaceScan(ctx, map[string]any{
					"phase": "activity_timestamp",
					"path":  path,
					"count": walkCount,
				})
			}
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				if path != planDir && shouldSkipPlanWorkspaceDir(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !isPlanWorkspaceActivityFile(path) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
			return nil
		},
	)
	return latest, err
}

func heartbeatPlanWorkspaceScan(ctx context.Context, details any) {
	if activity.IsActivity(ctx) {
		activity.RecordHeartbeat(ctx, details)
	}
}

func shouldSkipPlanWorkspaceDir(name string) bool {
	switch strings.TrimSpace(name) {
	case ".git",
		".pi",
		"node_modules",
		"dist",
		"build",
		"coverage",
		".cache",
		"tmp",
		"vendor",
		"generated":
		return true
	default:
		return false
	}
}

func isPlanWorkspaceActivityFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".json", ".yaml", ".yml", ".toml":
		return true
	default:
		return false
	}
}

func WorkspaceDocPathFromRoot(root, rel string) (string, error) {
	root = strings.Trim(strings.TrimSpace(filepath.ToSlash(root)), "/")
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if root == "" {
		return "", fmt.Errorf("root doc path is required")
	}
	if rel == "" || rel == "." {
		return root, nil
	}
	if strings.HasPrefix(rel, "../") || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("relative doc path %q escapes root", rel)
	}
	return path.Join(root, rel), nil
}

func (s *PlanWorkspaceSyncer) SyncWorkspaceDocIndex(
	ctx context.Context,
	workspace db.Workspace,
) ([]WorkspaceDocChange, error) {
	if s == nil || s.Queries == nil {
		return nil, errors.New("workspace doc sync requires queries")
	}
	rootDocPath := strings.Trim(strings.TrimSpace(workspace.RootDocPath), "/")
	if rootDocPath == "" {
		return nil, errors.New("workspace root doc path is required")
	}
	thoughtsRoot := strings.TrimSpace(s.Scanner.ThoughtsRoot)
	if thoughtsRoot == "" {
		return nil, errors.New("thoughts root is required")
	}
	fsRoot := filepath.Join(filepath.Clean(thoughtsRoot), filepath.FromSlash(rootDocPath))
	if info, err := os.Stat(fsRoot); err != nil || !info.IsDir() {
		if err != nil {
			return nil, fmt.Errorf("stat workspace doc root: %w", err)
		}
		return nil, fmt.Errorf("workspace doc root %s is not a directory", fsRoot)
	}

	seen := collections.NewSet[string]()
	changes := []WorkspaceDocChange{}
	walkCount := 0
	err := filepath.WalkDir(
		fsRoot,
		func(itemPath string, entry fs.DirEntry, walkErr error) error {
			walkCount++
			if walkCount%100 == 0 {
				heartbeatPlanWorkspaceScan(ctx, map[string]any{
					"phase": "workspace_doc_index",
					"path":  itemPath,
					"count": walkCount,
				})
			}
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() && itemPath != fsRoot &&
				shouldSkipPlanWorkspaceDir(entry.Name()) {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(fsRoot, itemPath)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				rel = "."
			}
			docPath, err := WorkspaceDocPathFromRoot(rootDocPath, rel)
			if err != nil {
				return err
			}
			seen.Add(docPath)
			info, err := entry.Info()
			if err != nil {
				return err
			}
			kind := WorkspaceDocKindFile
			if entry.IsDir() {
				kind = WorkspaceDocKindDir
			}
			sizeBytes := info.Size()
			var contentHash sql.NullString
			if !entry.IsDir() {
				hash, err := fileContentHash(itemPath)
				if err != nil {
					return err
				}
				contentHash = nullString(hash)
			}
			if err := s.Queries.UpsertWorkspaceDoc(ctx, db.UpsertWorkspaceDocParams{
				WorkspaceID: workspace.ID,
				DocPath:     docPath,
				RelPath:     rel,
				Kind:        string(kind),
				Title:       workspaceDocTitle(rel, entry),
				SizeBytes:   sizeBytes,
				MtimeUnix:   info.ModTime().Unix(),
				ContentHash: contentHash,
			}); err != nil {
				return err
			}
			changes = append(
				changes,
				WorkspaceDocChange{
					RelPath: rel,
					DocPath: docPath,
					Kind:    kind,
					Action:  WorkspaceDocActionUpsert,
				},
			)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	existing, err := s.Queries.ListWorkspaceDocs(ctx, workspace.ID)
	if err != nil {
		return nil, err
	}
	for _, item := range existing {
		if seen.Has(item.DocPath) {
			continue
		}
		if err := s.Queries.MarkWorkspaceDocDeleted(ctx, db.MarkWorkspaceDocDeletedParams{
			WorkspaceID: workspace.ID,
			DocPath:     item.DocPath,
		}); err != nil {
			return nil, err
		}
		changes = append(
			changes,
			WorkspaceDocChange{
				RelPath: item.RelPath,
				DocPath: item.DocPath,
				Kind:    WorkspaceDocKind(item.Kind),
				Action:  WorkspaceDocActionDelete,
			},
		)
	}
	return changes, nil
}

func fileContentHash(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

func workspaceDocTitle(rel string, entry fs.DirEntry) string {
	if rel == "." {
		return entry.Name()
	}
	base := path.Base(filepath.ToSlash(rel))
	if base == "." || base == "/" || base == "" {
		return entry.Name()
	}
	return base
}
