package agentchat

import (
	"context"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/CoreyCole/vamos/pkg/db"
)

type WorkbenchThread struct {
	ID        string
	Title     string
	PlanDir   string
	PlanLabel string
	UpdatedAt time.Time
}

type WorkbenchThreadGroup struct {
	PlanDir string
	Label   string
	Threads []WorkbenchThread
}

func (s *Service) ResolveThreadPlanDir(
	ctx context.Context,
	thread db.AgentThread,
) string {
	workspaceRoot := ""
	workspace, err := s.queries.GetSharedPrimaryWorkspaceForThread(ctx, thread.ID)
	if err == nil {
		workspaceRoot = workspace.RootDocPath
	}
	planDir, _ := s.threadPlanDir(ctx, thread.PlanDirRel, thread.Cwd, workspaceRoot)
	return planDir
}

func threadWorkbenchHref(threadID, artifact string) string {
	if strings.TrimSpace(artifact) == "" {
		return "/threads/" + threadID
	}
	return "/threads/" + threadID + "?artifact=" + url.QueryEscape(artifact)
}

func threadSearchMatch(title string) string {
	return "String(" + strconv.Quote(
		title,
	) + ").toLowerCase().includes($threadSearch.toLowerCase())"
}

func threadGroupSearchMatch(threads []WorkbenchThread) string {
	matches := make([]string, 0, len(threads))
	for _, thread := range threads {
		matches = append(matches, threadSearchMatch(thread.Title))
	}
	if len(matches) == 0 {
		return "false"
	}
	return "(" + strings.Join(matches, " || ") + ")"
}

func (s *Service) RenderWorkbenchThreadList(
	ctx context.Context,
	selectedID, artifact string,
) (templ.Component, error) {
	groups, err := s.ListWorkbenchThreads(ctx)
	if err != nil {
		return nil, err
	}
	return WorkbenchThreadList(groups, selectedID, artifact), nil
}

func (s *Service) ResolveSharedThreadPlanDir(
	ctx context.Context,
	threadID string,
) (string, error) {
	thread, err := s.queries.GetSharedAgentThread(ctx, strings.TrimSpace(threadID))
	if err != nil {
		return "", err
	}
	return thoughtsPlanKey(s.ResolveThreadPlanDir(ctx, thread)), nil
}

func thoughtsPlanKey(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	if i := strings.Index(path, "thoughts/"); i >= 0 {
		path = path[i:]
	}
	return filepath.ToSlash(planDirectoryRoot(path))
}

func (s *Service) FindSharedThreadForDoc(
	ctx context.Context,
	docPath string,
) (string, error) {
	want := thoughtsPlanKey(docPath)
	if want == "" {
		return "", nil
	}
	// Plan-home: prefer FK most-recent when the plan workspace exists.
	if rel, ok := s.lookupPlanDirRel(ctx, docPath); ok {
		if thread, found, err := s.MostRecentPlanHomeThread(ctx, rel, ""); err != nil {
			return "", err
		} else if found {
			return thread.ID, nil
		}
	}
	groups, err := s.ListWorkbenchThreads(ctx)
	if err != nil {
		return "", err
	}
	for _, group := range groups {
		plan := thoughtsPlanKey(group.PlanDir)
		if plan == "" || plan != want {
			continue
		}
		if len(group.Threads) == 0 {
			return "", nil
		}
		return group.Threads[0].ID, nil
	}
	return "", nil
}

func (s *Service) RenderSharedThreadChat(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "")
}

func (s *Service) ListWorkbenchThreads(
	ctx context.Context,
) ([]WorkbenchThreadGroup, error) {
	rows, err := s.queries.ListSharedAgentThreadsWithWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	groups := map[string]*WorkbenchThreadGroup{}
	for _, row := range rows {
		// Dual-read: prefer plan_dir_rel FK; fallback to cwd / workspace root.
		planDir, ok := s.threadPlanDir(
			ctx,
			row.PlanDirRel,
			row.Cwd,
			row.WorkspaceRootDocPath.String,
		)
		key := planDir
		label := "Ungrouped threads"
		if !ok {
			key = ""
			planDir = ""
		} else {
			label = filepath.Base(planDir)
		}
		group := groups[key]
		if group == nil {
			group = &WorkbenchThreadGroup{
				PlanDir: planDir,
				Label:   label,
				Threads: []WorkbenchThread{},
			}
			groups[key] = group
		}
		group.Threads = append(group.Threads, WorkbenchThread{
			ID:        row.ID,
			Title:     row.Title,
			PlanDir:   planDir,
			PlanLabel: label,
			UpdatedAt: row.UpdatedAt,
		})
	}
	out := make([]WorkbenchThreadGroup, 0, len(groups))
	for _, group := range groups {
		sort.SliceStable(group.Threads, func(i, j int) bool {
			return group.Threads[i].UpdatedAt.After(group.Threads[j].UpdatedAt)
		})
		out = append(out, *group)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].PlanDir == "" {
			return false
		}
		if out[j].PlanDir == "" {
			return true
		}
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out, nil
}
