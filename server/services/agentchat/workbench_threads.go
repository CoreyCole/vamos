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
	if planDir, ok := s.canonicalPlanDirFromSource(thread.Cwd); ok {
		return planDir
	}
	workspace, err := s.queries.GetSharedPrimaryWorkspaceForThread(ctx, thread.ID)
	if err != nil {
		return ""
	}
	planDir, _ := s.canonicalPlanDirFromSource(workspace.RootDocPath)
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

func (s *Service) RenderSharedThreadChat(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	thread, err := s.queries.GetSharedAgentThread(ctx, strings.TrimSpace(threadID))
	if err != nil {
		return nil, err
	}
	draft, err := s.GetThreadDraft(ctx, userEmail, thread.ID)
	if err != nil {
		return nil, err
	}
	live, cursor := s.buildLiveTranscript(thread.ID)
	args := EmbeddedFreeformPanelArgs{
		ThreadID:  thread.ID,
		HasThread: true,
		Cwd:       thread.Cwd,
		Transcript: TranscriptPaneState{
			Stable: []TranscriptMessage{},
			Live:   live,
			Cursor: cursor,
			Policy: s.defaultTranscriptRenderPolicy(),
		},
		ComposerAction: "@post('" + thoughtsThreadChatAction(
			thread.ID,
			"resume",
		) + "', {contentType: 'form'})",
		StreamURL:    thoughtsThreadChatAction(thread.ID, "stream") + "?since=0",
		InitialDraft: draft,
		DraftSaveAction: "@post('/agent-chat/thread/" + url.PathEscape(
			thread.ID,
		) + "/draft', {filterSignals: {include: /^chatDraft$/}})",
	}
	args.ComposerAction = "@post('" + thoughtsThreadChatAction(
		thread.ID,
		"resume",
	) + "?workbench_v2=1', {contentType: 'form'})"
	args.StreamURL = thoughtsThreadChatAction(
		thread.ID,
		"stream",
	) + "?since=0&workbench_v2=1"
	return SharedThreadChat(args), nil
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
		planDir, ok := s.canonicalPlanDirFromSource(row.Cwd)
		if !ok {
			planDir, ok = s.canonicalPlanDirFromSource(row.WorkspaceRootDocPath.String)
		}
		key := planDir
		label := "Ungrouped threads"
		if !ok {
			key = ""
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
