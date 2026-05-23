package workspaces

import (
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type ImplWorkspaceView struct {
	Row        db.ImplWorkspace
	Runtime    WorkspaceLifecycleSnapshot
	HasRuntime bool
	IsMain     bool
	Children   []ImplWorkspaceView
}

func BuildImplWorkspaceViews(
	rows []db.ImplWorkspace,
	runtime []WorkspaceLifecycleSnapshot,
	main WorkspaceLifecycleSnapshot,
) []ImplWorkspaceView {
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
		views = append(views, ImplWorkspaceView{
			Row: db.ImplWorkspace{
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
		})
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
		views = append(views, view)
	}
	return BuildImplWorkspaceTree(views)
}

func BuildImplWorkspaceTree(views []ImplWorkspaceView) []ImplWorkspaceView {
	byPlanDir := make(map[string]int, len(views))
	for i := range views {
		if dir := implWorkspaceViewPlanDirKey(views[i]); dir != "" {
			byPlanDir[dir] = i
		}
	}

	childrenByParent := make(map[int][]ImplWorkspaceView)
	childIndexes := make(map[int]bool)
	for i := range views {
		parentDir := parentPlanDirKey(implWorkspaceViewPlanDirKey(views[i]))
		parentIndex, ok := byPlanDir[parentDir]
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
