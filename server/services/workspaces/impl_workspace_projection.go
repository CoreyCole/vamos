package workspaces

import (
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type ImplWorkspaceView struct {
	Row            db.ImplWorkspace
	Runtime        WorkspaceLifecycleSnapshot
	HasRuntime     bool
	IsMain         bool
	Children       []ImplWorkspaceView
	ReleaseActions []ReleaseActionView
	Workflow       WorkspaceWorkflowSummary
	Cleanup        CleanupReadiness
	Diagnostics    WorkspaceLifecycleDiagnostic
	Plan           PlanWorkspaceView
}

type PlanWorkspaceProjectView struct {
	ProjectID string
	Role      string
	Label     string
}

type PlanWorkspaceImplBindingView struct {
	ProjectID     string
	Role          string
	WorkspaceSlug string
	CheckoutPath  string
	URL           string
	Status        string
	ImplWorkspace *db.ImplWorkspace
}

type PlanWorkspaceView struct {
	PlanDirRel  string
	Projects    []PlanWorkspaceProjectView
	Bindings    []PlanWorkspaceImplBindingView
	MatchedRole string
}

type ImplWorkspaceViewOption func(*implWorkspaceViewOptions)

type implWorkspaceViewOptions struct {
	releaseActions map[string][]ReleaseActionView
	workflows      map[string]WorkspaceWorkflowSummary
	syncDiagnostic WorkspaceSyncDiagnostic
}

func WithWorkspaceReleaseActions(actions map[string][]ReleaseActionView) ImplWorkspaceViewOption {
	return func(opts *implWorkspaceViewOptions) { opts.releaseActions = actions }
}

func WithWorkspaceWorkflowSummaries(summaries map[string]WorkspaceWorkflowSummary) ImplWorkspaceViewOption {
	return func(opts *implWorkspaceViewOptions) { opts.workflows = summaries }
}

func WithWorkspaceSyncDiagnostic(sync WorkspaceSyncDiagnostic) ImplWorkspaceViewOption {
	return func(opts *implWorkspaceViewOptions) { opts.syncDiagnostic = sync }
}

func BuildImplWorkspaceViews(
	rows []db.ImplWorkspace,
	runtime []WorkspaceLifecycleSnapshot,
	main WorkspaceLifecycleSnapshot,
	opts ...ImplWorkspaceViewOption,
) []ImplWorkspaceView {
	options := buildImplWorkspaceViewOptions(opts...)
	runtimeBySlug := make(map[string]WorkspaceLifecycleSnapshot, len(runtime))
	runtimeByPath := make(map[string]WorkspaceLifecycleSnapshot, len(runtime))
	for _, snap := range runtime {
		if slug := strings.TrimSpace(snap.Workspace.Slug); slug != "" {
			runtimeBySlug[slug] = snap
		}
		if path := cleanPathKey(snap.Workspace.CheckoutPath); path != "" {
			runtimeByPath[path] = snap
		}
	}

	views := make([]ImplWorkspaceView, 0, len(rows)+1)
	if strings.TrimSpace(main.Workspace.Slug) != "" {
		view := ImplWorkspaceView{
			Row: db.ImplWorkspace{
				ProjectID:     main.Workspace.ProjectID,
				CheckoutRole:  string(main.Workspace.CheckoutRole),
				WorkspaceSlug: main.Workspace.Slug,
				CheckoutPath:  main.Workspace.CheckoutPath,
				DisplayName:   workspaceNavLabel(main.Workspace),
				Host:          main.Workspace.Host,
				Url:           main.Workspace.URL,
				Status:        string(ImplWorkspaceStatusActive),
			},
			Runtime:    main,
			HasRuntime: true,
			IsMain:     true,
		}
		view.Diagnostics = BuildWorkspaceLifecycleDiagnostic(view.Row, view.Runtime, view.HasRuntime, options.syncDiagnostic)
		applyImplWorkspaceViewOptions(&view, options)
		views = append(views, view)
	}

	for _, row := range rows {
		if row.WorkspaceSlug == mainWorkspaceSlug {
			continue
		}
		view := ImplWorkspaceView{Row: row}
		if snap, ok := runtimeBySlug[strings.TrimSpace(row.WorkspaceSlug)]; ok {
			view.Runtime = snap
			view.HasRuntime = true
		} else if snap, ok := runtimeByPath[cleanPathKey(row.CheckoutPath)]; ok {
			view.Runtime = snap
			view.HasRuntime = true
		} else {
			view.Runtime = snapshotFromState(Workspace{
				ProjectID:    row.ProjectID,
				CheckoutRole: CheckoutRole(row.CheckoutRole),
				Slug:         row.WorkspaceSlug,
				DisplayName:  row.DisplayName,
				CheckoutPath: row.CheckoutPath,
				Host:         row.Host,
				URL:          row.Url,
				Status:       StatusStopped,
				Branch:       nullStringValue(row.Branch),
				Commit:       nullStringValue(row.CommitHash),
				Stack: StackSummary{
					Branch:       nullStringValue(row.Branch),
					TopBranch:    nullStringValue(row.TopBranch),
					BottomBranch: nullStringValue(row.BottomBranch),
					BottomParent: nullStringValue(row.BottomParentBranch),
					TrunkBranch:  nullStringValue(row.TrunkBranch),
					BaseBranch:   nullStringValue(row.BaseBranch),
					AheadCount:   int(row.AheadCount),
					BehindCount:  int(row.BehindCount),
					Detail:       nullStringValue(row.GitDetail),
				},
			}, WorkspaceLifecycleState{})
		}
		view.Diagnostics = BuildWorkspaceLifecycleDiagnostic(view.Row, view.Runtime, view.HasRuntime, options.syncDiagnostic)
		applyImplWorkspaceViewOptions(&view, options)
		views = append(views, view)
	}
	tree := BuildImplWorkspaceTree(views)
	applyCleanupReadiness(tree)
	return tree
}

func buildImplWorkspaceViewOptions(opts ...ImplWorkspaceViewOption) implWorkspaceViewOptions {
	var options implWorkspaceViewOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

func applyImplWorkspaceViewOptions(view *ImplWorkspaceView, opts implWorkspaceViewOptions) {
	if view == nil {
		return
	}
	slug := strings.TrimSpace(view.Row.WorkspaceSlug)
	if slug == "" {
		slug = strings.TrimSpace(view.Runtime.Workspace.Slug)
	}
	if opts.releaseActions != nil {
		view.ReleaseActions = append([]ReleaseActionView{}, opts.releaseActions[slug]...)
	}
	if opts.workflows != nil {
		view.Workflow = opts.workflows[slug]
	}
}

func applyOptionsToImplWorkspaceViews(views []ImplWorkspaceView, opts ...ImplWorkspaceViewOption) []ImplWorkspaceView {
	options := buildImplWorkspaceViewOptions(opts...)
	out := append([]ImplWorkspaceView(nil), views...)
	var apply func([]ImplWorkspaceView)
	apply = func(items []ImplWorkspaceView) {
		for i := range items {
			applyImplWorkspaceViewOptions(&items[i], options)
			items[i].Cleanup = workspaceCleanupReadiness(items[i])
			items[i].Diagnostics = BuildWorkspaceLifecycleDiagnostic(items[i].Row, items[i].Runtime, items[i].HasRuntime, options.syncDiagnostic)
			items[i].Children = append([]ImplWorkspaceView(nil), items[i].Children...)
			apply(items[i].Children)
		}
	}
	apply(out)
	return out
}

func applyCleanupReadiness(views []ImplWorkspaceView) {
	for i := range views {
		views[i].Cleanup = workspaceCleanupReadiness(views[i])
		applyCleanupReadiness(views[i].Children)
	}
}

func AttachWorkspaceDiagnostics(views []ImplWorkspaceView, sync WorkspaceSyncDiagnostic) []ImplWorkspaceView {
	out := append([]ImplWorkspaceView(nil), views...)
	var attach func([]ImplWorkspaceView)
	attach = func(items []ImplWorkspaceView) {
		for i := range items {
			items[i].Diagnostics = BuildWorkspaceLifecycleDiagnostic(items[i].Row, items[i].Runtime, items[i].HasRuntime, sync)
			items[i].Children = append([]ImplWorkspaceView(nil), items[i].Children...)
			attach(items[i].Children)
		}
	}
	attach(out)
	return out
}

func orderReleaseLaneViewsFirst(
	views []ImplWorkspaceView,
	lanes []ReleaseLaneWorkspace,
) []ImplWorkspaceView {
	if len(views) == 0 || len(lanes) == 0 {
		return views
	}
	laneBySlug := make(map[string]ReleaseLaneWorkspace, len(lanes))
	for _, lane := range lanes {
		if lane.Slug != "" {
			laneBySlug[lane.Slug] = lane
		}
	}
	mainViews := make([]ImplWorkspaceView, 0, 1)
	stageViews := make([]ImplWorkspaceView, 0, 1)
	otherViews := make([]ImplWorkspaceView, 0, len(views))
	for _, view := range views {
		slug := workspaceViewSlug(view)
		lane, isLane := laneBySlug[slug]
		switch {
		case view.IsMain || slug == mainWorkspaceSlug || lane.Role == ReleaseLaneRoleMain:
			mainViews = append(mainViews, view)
		case isLane && lane.Role == ReleaseLaneRoleStage:
			stageViews = append(stageViews, view)
		default:
			otherViews = append(otherViews, view)
		}
	}
	out := append(mainViews, stageViews...)
	out = append(out, otherViews...)
	return out
}

func BuildImplWorkspaceTree(views []ImplWorkspaceView) []ImplWorkspaceView {
	parentsByPlanDir := make(map[string][]int, len(views))
	parentsByPlanProject := make(map[planDirProjectKey]int, len(views))
	for i := range views {
		dir := implWorkspaceViewPlanDirKey(views[i])
		if dir == "" || parentPlanDirKey(dir) != "" {
			continue
		}
		parentsByPlanDir[dir] = append(parentsByPlanDir[dir], i)
		projectID := implWorkspaceViewProjectID(views[i])
		if projectID != "" {
			parentsByPlanProject[planDirProjectKey{PlanDir: dir, ProjectID: projectID}] = i
		}
	}

	childrenByParent := make(map[int][]ImplWorkspaceView)
	childIndexes := make(map[int]bool)
	for i := range views {
		planDir := implWorkspaceViewPlanDirKey(views[i])
		parentDir := parentPlanDirKey(planDir)
		if parentDir == "" {
			continue
		}
		parentIndex, ok := parentIndexForReviewWorkspace(views[i], parentDir, parentsByPlanDir, parentsByPlanProject)
		if !ok || parentIndex == i {
			continue
		}
		childrenByParent[parentIndex] = append(childrenByParent[parentIndex], views[i])
		childIndexes[i] = true
	}

	out := make([]ImplWorkspaceView, 0, len(views))
	for i, view := range views {
		if childIndexes[i] {
			continue
		}
		view.Children = childrenByParent[i]
		out = append(out, view)
	}
	return out
}

type planDirProjectKey struct {
	PlanDir   string
	ProjectID string
}

func parentIndexForReviewWorkspace(
	view ImplWorkspaceView,
	parentDir string,
	parentsByPlanDir map[string][]int,
	parentsByPlanProject map[planDirProjectKey]int,
) (int, bool) {
	projectID := implWorkspaceViewProjectID(view)
	if projectID != "" {
		if idx, ok := parentsByPlanProject[planDirProjectKey{PlanDir: parentDir, ProjectID: projectID}]; ok {
			return idx, true
		}
	}
	candidates := parentsByPlanDir[parentDir]
	if len(candidates) != 1 {
		return 0, false
	}
	return candidates[0], true
}

func implWorkspaceViewProjectID(view ImplWorkspaceView) string {
	if projectID := strings.TrimSpace(view.Row.ProjectID); projectID != "" {
		return projectID
	}
	return strings.TrimSpace(view.Runtime.Workspace.ProjectID)
}

func implWorkspaceViewPlanDirKey(view ImplWorkspaceView) string {
	if view.Row.PlanDirRel.Valid {
		if key := normalizePlanDirKey(view.Row.PlanDirRel.String); key != "" {
			return key
		}
	}
	if view.Row.PlanDir.Valid {
		return normalizePlanDirKey(view.Row.PlanDir.String)
	}
	return ""
}

func parentPlanDirKey(planDir string) string {
	planDir = normalizePlanDirKey(planDir)
	if planDir == "" {
		return ""
	}
	marker := "/reviews/"
	idx := strings.Index(planDir, marker)
	if idx < 0 {
		return ""
	}
	return strings.Trim(planDir[:idx], "/")
}

func normalizePlanDirKey(planDir string) string {
	planDir = strings.TrimSpace(strings.ReplaceAll(planDir, "\\", "/"))
	planDir = strings.Trim(planDir, "/")
	if planDir == "" {
		return ""
	}
	if idx := strings.Index(planDir, "/thoughts/"); idx >= 0 {
		planDir = planDir[idx+len("/thoughts/"):]
	}
	if strings.HasPrefix(planDir, "thoughts/") {
		planDir = strings.TrimPrefix(planDir, "thoughts/")
	}
	return strings.Trim(planDir, "/")
}

func ImplViewsToNavWorkspaces(views []ImplWorkspaceView) []Workspace {
	out := make([]Workspace, 0, len(views))
	appendView := func(view ImplWorkspaceView) {
		if view.Runtime.Workspace.Slug != "" {
			out = append(out, view.Runtime.Workspace)
			return
		}
		if view.Row.Status == string(ImplWorkspaceStatusActive) {
			out = append(out, Workspace{
				ProjectID:    view.Row.ProjectID,
				CheckoutRole: CheckoutRole(view.Row.CheckoutRole),
				Slug:         view.Row.WorkspaceSlug,
				DisplayName:  view.Row.DisplayName,
				CheckoutPath: view.Row.CheckoutPath,
				Status:       StatusStopped,
			})
		}
	}
	var walk func([]ImplWorkspaceView)
	walk = func(items []ImplWorkspaceView) {
		for _, view := range items {
			appendView(view)
			walk(view.Children)
		}
	}
	walk(views)
	return out
}

func lifecycleSnapshotsToImplViews(
	items []WorkspaceLifecycleSnapshot,
) []ImplWorkspaceView {
	views := make([]ImplWorkspaceView, 0, len(items))
	for _, snap := range items {
		views = append(views, ImplWorkspaceView{
			Row: db.ImplWorkspace{
				ProjectID:     snap.Workspace.ProjectID,
				CheckoutRole:  string(snap.Workspace.CheckoutRole),
				WorkspaceSlug: snap.Workspace.Slug,
				CheckoutPath:  snap.Workspace.CheckoutPath,
				DisplayName:   workspaceNavLabel(snap.Workspace),
				Host:          snap.Workspace.Host,
				Url:           snap.Workspace.URL,
				Status:        string(ImplWorkspaceStatusActive),
			},
			Runtime:    snap,
			HasRuntime: true,
			IsMain:     snap.Workspace.IsMain || snap.Workspace.Slug == mainWorkspaceSlug,
		})
	}
	return views
}

func splitMainSnapshot(
	runtime []WorkspaceLifecycleSnapshot,
) (WorkspaceLifecycleSnapshot, []WorkspaceLifecycleSnapshot) {
	main := WorkspaceLifecycleSnapshot{}
	nonMain := make([]WorkspaceLifecycleSnapshot, 0, len(runtime))
	for _, snap := range runtime {
		if snap.Workspace.IsMain || snap.Workspace.Slug == mainWorkspaceSlug {
			main = snap
			continue
		}
		nonMain = append(nonMain, snap)
	}
	return main, nonMain
}
