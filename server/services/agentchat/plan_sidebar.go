package agentchat

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/collections"
	"github.com/CoreyCole/vamos/pkg/db"
)

const (
	defaultPlanSidebarTargetID  = "agent-chat-thread-sidebar"
	planSidebarSourceDiscovered = "discovered"
	planSidebarSourceSession    = "session"
	planSidebarSourceThread     = "thread"
	jsonlExtension              = ".jsonl"
)

func (s *Service) BuildPlanSidebarState(
	ctx context.Context,
	input PlanSidebarInput,
) (PlanSidebarState, error) {
	activePlan, _ := s.canonicalPlanDirFromSource(input.ActivePlanDir)
	if activePlan == "" {
		activePlan = s.resolveActivePlanDir(ctx, input)
	}
	sources, err := s.collectPlanSidebarSources(ctx, input.UserEmail)
	if err != nil {
		return PlanSidebarState{}, err
	}
	nodes := s.buildPlanSidebarTreeForUser(ctx, input.UserEmail, sources, activePlan)
	return PlanSidebarState{
		TargetID:          planSidebarTargetOrDefault(input),
		DrawerTitle:       "Plan workspaces",
		ActivePlanDir:     activePlan,
		ActiveWorkspaceID: strings.TrimSpace(input.ActiveWorkspaceID),
		HasSelection: activePlan != "" ||
			strings.TrimSpace(input.ActiveWorkspaceID) != "" ||
			strings.TrimSpace(input.ActiveThreadID) != "",
		Nodes: nodes,
	}, nil
}

func (s *Service) resolveActivePlanDir(
	ctx context.Context,
	input PlanSidebarInput,
) string {
	userEmail := strings.TrimSpace(input.UserEmail)
	if userEmail == "" {
		return ""
	}

	if planDir := s.resolveActiveThreadPlanDir(
		ctx,
		userEmail,
		input.ActiveThreadID,
	); planDir != "" {
		return planDir
	}
	return s.resolveActiveWorkspacePlanDir(ctx, userEmail, input.ActiveWorkspaceID)
}

func (s *Service) resolveActiveThreadPlanDir(
	ctx context.Context,
	userEmail, threadID string,
) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	thread, err := s.queries.GetAgentThreadForUser(ctx, db.GetAgentThreadForUserParams{
		ID:        threadID,
		UserEmail: userEmail,
	})
	if err != nil {
		return ""
	}
	if planDir, ok := s.canonicalPlanDirFromSource(thread.Cwd); ok {
		return planDir
	}
	workspace, ok, err := s.ResolvePrimaryWorkspaceForThread(ctx, userEmail, thread.ID)
	if err != nil || !ok {
		return ""
	}
	return s.resolveActiveWorkspacePlanDir(ctx, userEmail, workspace.ID)
}

func (s *Service) resolveActiveWorkspacePlanDir(
	ctx context.Context,
	userEmail, workspaceID string,
) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ""
	}
	workspace, err := s.GetWorkspaceForUser(ctx, userEmail, workspaceID)
	if err != nil {
		return ""
	}
	planDir, ok := s.canonicalPlanDirFromSource(workspace.RootDocPath)
	if !ok {
		return ""
	}
	return planDir
}

func (s *Service) collectPlanSidebarSources(
	ctx context.Context,
	userEmail string,
) ([]PlanSidebarSource, error) {
	discovered, err := s.collectDiscoveredPlanSidebarSources(ctx)
	if err != nil {
		return nil, err
	}
	overlay, err := s.collectUserPlanSidebarOverlaySources(ctx, userEmail)
	if err != nil {
		return nil, err
	}
	return mergePlanSidebarSources(discovered, overlay), nil
}

func (s *Service) collectDiscoveredPlanSidebarSources(
	ctx context.Context,
) ([]PlanSidebarSource, error) {
	rows, err := s.queries.ListCurrentPlanWorkspaces(ctx)
	if err != nil {
		return nil, err
	}

	sources := make([]PlanSidebarSource, 0, len(rows))
	for _, row := range rows {
		planDir, ok := s.canonicalPlanDirFromSource(row.PlanDir)
		if !ok {
			continue
		}
		sources = append(sources, PlanSidebarSource{
			PlanDir:    planDir,
			PlanDirRel: row.PlanDirRel,
			Source:     planSidebarSourceDiscovered,
			Title:      row.Label,
			UpdatedAt:  row.ArtifactUpdatedAt,
		})
	}
	return sources, nil
}

func (s *Service) collectUserPlanSidebarOverlaySources(
	ctx context.Context,
	userEmail string,
) ([]PlanSidebarSource, error) {
	userEmail = strings.TrimSpace(userEmail)
	if userEmail == "" {
		return []PlanSidebarSource{}, nil
	}

	sources := []PlanSidebarSource{}
	seen := collections.NewSet[string]()

	sessions, err := s.queries.ListAgentSessionsForUser(
		ctx,
		sql.NullString{String: userEmail, Valid: true},
	)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		planDir, ok := s.canonicalPlanDirFromSource(session.InferredPlanDir.String)
		if !ok {
			continue
		}
		source := PlanSidebarSource{
			PlanDir:     planDir,
			PlanDirRel:  s.planSidebarRel(planDir),
			WorkspaceID: session.WorkspaceID.String,
			ThreadID:    session.ThreadID.String,
			SessionID:   session.ID,
			Source:      planSidebarSourceSession,
			Title:       sessionTitleForPlanSidebar(session),
			UpdatedAt:   session.UpdatedAt,
		}
		addPlanSidebarSource(&sources, seen, source)
	}

	threads, err := s.queries.ListAgentThreadsForUserWithWorkspace(ctx, userEmail)
	if err != nil {
		return nil, err
	}
	for _, row := range threads {
		candidates := []string{row.WorkspaceRootDocPath.String, row.Cwd}
		for _, candidate := range candidates {
			planDir, ok := s.canonicalPlanDirFromSource(candidate)
			if !ok {
				continue
			}
			source := PlanSidebarSource{
				PlanDir:     planDir,
				PlanDirRel:  s.planSidebarRel(planDir),
				WorkspaceID: row.PrimaryWorkspaceID.String,
				ThreadID:    row.ID,
				Source:      planSidebarSourceThread,
				Title:       row.Title,
				UpdatedAt:   row.UpdatedAt,
			}
			addPlanSidebarSource(&sources, seen, source)
			break
		}
	}

	return sources, nil
}

func mergePlanSidebarSources(
	discovered, overlay []PlanSidebarSource,
) []PlanSidebarSource {
	merged := make([]PlanSidebarSource, 0, len(discovered)+len(overlay))
	seen := collections.NewSet[string]()
	for _, source := range discovered {
		addPlanSidebarSource(&merged, seen, source)
	}
	for _, source := range overlay {
		addPlanSidebarSource(&merged, seen, source)
	}
	return merged
}

func (s *Service) canonicalPlanDirFromSource(raw string) (string, bool) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", false
	}
	if !filepath.IsAbs(clean) && s.projectRoot != "" {
		clean = filepath.Join(s.projectRoot, clean)
	}
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	clean = filepath.Clean(clean)

	if hasFileExtension(clean) {
		clean = filepath.Dir(clean)
	}

	topPlanDir := planDirectoryRoot(clean)
	if topPlanDir == "" {
		return "", false
	}
	if s.thoughtsRoot != "" && !pathWithinRoot(clean, s.thoughtsRoot) {
		return "", false
	}
	if s.projectRoot != "" {
		projectThoughts := filepath.Join(s.projectRoot, "thoughts")
		if pathWithinRoot(projectThoughts, s.projectRoot) &&
			(s.thoughtsRoot == "" || pathWithinRoot(s.thoughtsRoot, projectThoughts)) &&
			!pathWithinRoot(clean, projectThoughts) {
			return "", false
		}
	}
	if !pathWithinRoot(clean, topPlanDir) {
		return filepath.Clean(topPlanDir), true
	}
	return clean, true
}

func (s *Service) buildPlanSidebarTree(
	sources []PlanSidebarSource,
	activePlanDir string,
) []PlanSidebarNode {
	return s.buildPlanSidebarTreeForUser(context.Background(), "", sources, activePlanDir)
}

func (s *Service) buildPlanSidebarTreeForUser(
	ctx context.Context,
	userEmail string,
	sources []PlanSidebarSource,
	activePlanDir string,
) []PlanSidebarNode {
	nodeByDir := map[string]*PlanSidebarNode{}
	directSources := map[string][]PlanSidebarSource{}
	seenSource := collections.NewSet[string]()

	for _, source := range sources {
		planDir, ok := s.canonicalPlanDirFromSource(source.PlanDir)
		if !ok {
			continue
		}
		source.PlanDir = planDir
		key := planSidebarSourceKey(source)
		if seenSource.Has(key) {
			continue
		}
		seenSource.Add(key)
		directSources[planDir] = append(directSources[planDir], source)
		for _, dir := range planSidebarAncestorDirs(planDir) {
			if _, ok := nodeByDir[dir]; !ok {
				nodeByDir[dir] = s.newPlanSidebarNode(dir)
			}
		}
	}

	activePlanDir = filepath.Clean(strings.TrimSpace(activePlanDir))
	for dir, node := range nodeByDir {
		if href := s.planSidebarPlanHref(ctx, userEmail, dir); href != "" {
			node.Href = href
		}
		for _, source := range directSources[dir] {
			applyPlanSidebarSource(node, source)
		}
		if activePlanDir != "" && sameFilesystemPath(dir, activePlanDir) {
			node.Active = true
		}
	}

	childrenByParent := map[string][]string{}
	rootDirs := []string{}
	for dir := range nodeByDir {
		parentDir := planSidebarParentDir(dir, nodeByDir)
		if parentDir == "" {
			rootDirs = append(rootDirs, dir)
			continue
		}
		childrenByParent[parentDir] = append(childrenByParent[parentDir], dir)
	}

	nodes := make([]PlanSidebarNode, 0, len(rootDirs))
	for _, rootDir := range rootDirs {
		node := buildPlanSidebarNodeFromMap(rootDir, nodeByDir, childrenByParent)
		aggregatePlanSidebarNode(&node, activePlanDir)
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return planSidebarNodeLess(nodes[i], nodes[j])
	})
	return nodes
}

func buildPlanSidebarNodeFromMap(
	dir string,
	nodeByDir map[string]*PlanSidebarNode,
	childrenByParent map[string][]string,
) PlanSidebarNode {
	node := *nodeByDir[dir]
	node.Children = nil
	for _, childDir := range childrenByParent[dir] {
		node.Children = append(
			node.Children,
			buildPlanSidebarNodeFromMap(childDir, nodeByDir, childrenByParent),
		)
	}
	return node
}

func (s *Service) newPlanSidebarNode(planDir string) *PlanSidebarNode {
	label, _ := formatSidebarGroupDisplay(filepath.Base(filepath.Clean(planDir)))
	if strings.TrimSpace(label) == "" {
		label = filepath.Base(filepath.Clean(planDir))
	}
	return &PlanSidebarNode{
		Key:        "plan:" + planDir,
		PlanDir:    planDir,
		PlanDirRel: s.planSidebarRel(planDir),
		Label:      label,
		Href:       s.planSidebarPlanHref(context.Background(), "", planDir),
		Depth:      planSidebarDepth(planDir),
	}
}

func (s *Service) planSidebarRel(planDir string) string {
	if s.thoughtsRoot != "" {
		if rel, err := filepath.Rel(s.thoughtsRoot, planDir); err == nil && rel != "." {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(filepath.Clean(planDir))
}

func aggregatePlanSidebarNode(node *PlanSidebarNode, activePlanDir string) {
	node.AggregateLatestAt = node.DirectLatestAt
	node.AggregateCount = node.DirectCount
	for i := range node.Children {
		aggregatePlanSidebarNode(&node.Children[i], activePlanDir)
		child := &node.Children[i]
		node.AggregateCount += child.AggregateCount
		if child.AggregateLatestAt.After(node.AggregateLatestAt) {
			node.AggregateLatestAt = child.AggregateLatestAt
		}
		if child.Active || child.Expanded {
			node.Expanded = true
		}
	}
	if activePlanDir != "" && pathWithinRoot(activePlanDir, node.PlanDir) {
		node.Expanded = true
	}
	sort.SliceStable(node.Children, func(i, j int) bool {
		return planSidebarNodeLess(node.Children[i], node.Children[j])
	})
}

func planSidebarTimestamp(node PlanSidebarNode) time.Time {
	if node.Expanded {
		return node.DirectLatestAt
	}
	return node.AggregateLatestAt
}

func applyPlanSidebarSource(node *PlanSidebarNode, source PlanSidebarSource) {
	node.DirectCount++
	if source.UpdatedAt.After(node.DirectLatestAt) {
		node.DirectLatestAt = source.UpdatedAt
		if node.LatestUserActivityAt.IsZero() {
			node.LatestThreadID = ""
			node.LatestSessionID = ""
			node.LatestSourceLabel = planSidebarSourceLabel(source)
		}
	}

	href := planSidebarSourceHref(source)
	if href == "" {
		return
	}
	if !node.LatestUserActivityAt.IsZero() &&
		!source.UpdatedAt.After(node.LatestUserActivityAt) {
		return
	}
	node.LatestUserActivityAt = source.UpdatedAt
	node.LatestThreadID = strings.TrimSpace(source.ThreadID)
	node.LatestSessionID = strings.TrimSpace(source.SessionID)
	node.LatestSourceLabel = planSidebarSourceLabel(source)
	node.Href = href
}

func addPlanSidebarSource(
	sources *[]PlanSidebarSource,
	seen collections.Set[string],
	source PlanSidebarSource,
) {
	key := planSidebarSourceKey(source)
	if seen.Has(key) {
		return
	}
	seen.Add(key)
	*sources = append(*sources, source)
}

func planSidebarSourceKey(source PlanSidebarSource) string {
	planKey := strings.TrimSpace(source.PlanDirRel)
	if planKey == "" {
		planKey = source.PlanDir
	}
	return strings.Join([]string{
		planKey,
		source.WorkspaceID,
		source.ThreadID,
		source.SessionID,
		source.Source,
	}, "\x00")
}

func sessionTitleForPlanSidebar(session db.AgentSession) string {
	if session.SessionID.Valid && strings.TrimSpace(session.SessionID.String) != "" {
		return strings.TrimSpace(session.SessionID.String)
	}
	if session.SessionPath.Valid && strings.TrimSpace(session.SessionPath.String) != "" {
		return strings.TrimSuffix(
			filepath.Base(session.SessionPath.String),
			filepath.Ext(session.SessionPath.String),
		)
	}
	return strings.TrimSpace(session.ID)
}

func planSidebarSourceLabel(source PlanSidebarSource) string {
	switch source.Source {
	case planSidebarSourceDiscovered:
		return "Artifact"
	case planSidebarSourceSession:
		return "Pi"
	case planSidebarSourceThread:
		return "Agent Chat"
	default:
		return strings.TrimSpace(source.Source)
	}
}

func (s *Service) planSidebarPlanHref(
	ctx context.Context,
	userEmail, planDir string,
) string {
	_ = ctx
	if strings.TrimSpace(userEmail) == "" {
		return ""
	}
	return s.thoughtsHrefForPlanDir(planDir)
}

func (s *Service) thoughtsHrefForPlanDir(planDir string) string {
	planDir = strings.TrimSpace(planDir)
	if planDir == "" {
		return ""
	}
	return thoughtsDocRedirectURLForRoot(s.thoughtsRoot, planDir, nil)
}

func planSidebarSourceHref(source PlanSidebarSource) string {
	_ = source
	return ""
}

func planSidebarTargetOrDefault(input PlanSidebarInput) string {
	return defaultPlanSidebarTargetID
}

func planSidebarTargetID(state PlanSidebarState) string {
	if targetID := strings.TrimSpace(state.TargetID); targetID != "" {
		return targetID
	}
	return defaultPlanSidebarTargetID
}

func planSidebarDrawerTitle(state PlanSidebarState) string {
	if title := strings.TrimSpace(state.DrawerTitle); title != "" {
		return title
	}
	return "Plan workspaces"
}

func planSidebarTimestampLabel(node PlanSidebarNode) string {
	if timestamp := planSidebarTimestamp(node); !timestamp.IsZero() {
		return formatWorkspaceEventTime(timestamp)
	}
	if node.Expanded && len(node.Children) > 0 && node.AggregateCount > 0 {
		return "No direct sessions"
	}
	return ""
}

func planSidebarCountLabel(node PlanSidebarNode) string {
	count := node.AggregateCount
	if node.Expanded {
		count = node.DirectCount
	}
	if count <= 0 {
		return ""
	}
	label := "source"
	if count != 1 {
		label = "sources"
	}
	if node.Expanded && node.AggregateCount > node.DirectCount {
		return fmt.Sprintf("%d direct · %d total", node.DirectCount, node.AggregateCount)
	}
	return fmt.Sprintf("%d %s", count, label)
}

func planSidebarNodeIsLeaf(node PlanSidebarNode) bool {
	return len(node.Children) == 0
}

func planSidebarDepthPaddingClass(depth int) string {
	const (
		firstNestedPlanSidebarDepth  = 1
		secondNestedPlanSidebarDepth = 2
		thirdNestedPlanSidebarDepth  = 3
	)
	switch {
	case depth <= 0:
		return "pl-2"
	case depth == firstNestedPlanSidebarDepth:
		return "pl-5"
	case depth == secondNestedPlanSidebarDepth:
		return "pl-8"
	case depth == thirdNestedPlanSidebarDepth:
		return "pl-11"
	default:
		return "pl-14"
	}
}

func planSidebarNodeOpenExpression(node PlanSidebarNode) string {
	wrapped := strconv.Quote("|" + planSidebarNodeSignalKey(node) + "|")
	return "$agentChatSidebarOpenGroups.includes(" + wrapped + ")"
}

func planSidebarNodeSetExpression(node PlanSidebarNode) string {
	key := strconv.Quote(planSidebarNodeSignalKey(node))
	wrapped := strconv.Quote("|" + planSidebarNodeSignalKey(node) + "|")
	return "($agentChatSidebarOpenGroups.includes(" + wrapped + ") ? " +
		"$agentChatSidebarOpenGroups = $agentChatSidebarOpenGroups.replace(" + wrapped + ", '|') : " +
		"$agentChatSidebarOpenGroups = ($agentChatSidebarOpenGroups || '|') + " + key + " + '|')"
}

func planSidebarNodeSignalKey(node PlanSidebarNode) string {
	return strings.TrimSpace(node.Key)
}

func planSidebarAncestorDirs(planDir string) []string {
	planDir = filepath.Clean(strings.TrimSpace(planDir))
	top := planDirectoryRoot(planDir)
	if top == "" {
		return []string{}
	}
	dirs := []string{filepath.Clean(top)}
	if sameFilesystemPath(planDir, top) {
		return dirs
	}
	rel, err := filepath.Rel(top, planDir)
	if err != nil || strings.TrimSpace(rel) == "" || rel == "." {
		return dirs
	}
	current := filepath.Clean(top)
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		dirs = append(dirs, current)
	}
	return dirs
}

func planSidebarParentDir(dir string, nodeByDir map[string]*PlanSidebarNode) string {
	dir = filepath.Clean(strings.TrimSpace(dir))
	for {
		parent := filepath.Dir(dir)
		if parent == dir || parent == "." || parent == string(filepath.Separator) {
			return ""
		}
		if _, ok := nodeByDir[parent]; ok {
			return parent
		}
		dir = parent
	}
}

func planSidebarDepth(planDir string) int {
	top := planDirectoryRoot(planDir)
	if top == "" || sameFilesystemPath(top, planDir) {
		return 0
	}
	rel, err := filepath.Rel(top, planDir)
	if err != nil || strings.TrimSpace(rel) == "" || rel == "." {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}

func planSidebarNodeLess(left, right PlanSidebarNode) bool {
	if !left.AggregateLatestAt.Equal(right.AggregateLatestAt) {
		return left.AggregateLatestAt.After(right.AggregateLatestAt)
	}
	return strings.ToLower(left.Label) < strings.ToLower(right.Label)
}

func hasFileExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md",
		".markdown",
		".txt",
		".json",
		jsonlExtension,
		".yaml",
		".yml",
		".go",
		".templ":
		return true
	default:
		return false
	}
}
