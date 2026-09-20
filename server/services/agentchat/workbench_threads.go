package agentchat

import (
	"context"
	"net/url"
	"os"
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
	return s.thoughtsSharedThreadKey(s.ResolveThreadPlanDir(ctx, thread)), nil
}

func thoughtsPlanKey(raw string) string {
	path := filepath.ToSlash(strings.TrimSpace(raw))
	if i := strings.Index(path, "thoughts/"); i >= 0 {
		path = path[i:]
	}
	return filepath.ToSlash(planDirectoryRoot(path))
}

// thoughtsSharedThreadKey is the plan-home identity for a doc: classic
// thoughts/<user>/plans/<id>, or the nearest AGENTS.md desk under thoughtsRoot.
func (s *Service) thoughtsSharedThreadKey(docPath string) string {
	if key := thoughtsPlanKey(docPath); key != "" {
		return key
	}
	return s.thoughtsAgentsDeskKey(docPath)
}

func (s *Service) thoughtsAgentsDeskKey(docPath string) string {
	if s == nil || strings.TrimSpace(s.thoughtsRoot) == "" {
		return ""
	}
	path := filepath.ToSlash(strings.TrimSpace(docPath))
	if path == "" {
		return ""
	}
	root, err := filepath.Abs(s.thoughtsRoot)
	if err != nil {
		return ""
	}
	root = filepath.Clean(root)
	var abs string
	if filepath.IsAbs(filepath.FromSlash(path)) {
		abs = filepath.Clean(filepath.FromSlash(path))
	} else {
		rel := path
		if i := strings.Index(path, "thoughts/"); i >= 0 {
			rel = strings.TrimPrefix(path[i:], "thoughts/")
		}
		abs = filepath.Join(root, filepath.FromSlash(rel))
	}
	dir := abs
	if info, err := os.Stat(abs); err == nil {
		if !info.IsDir() {
			dir = filepath.Dir(abs)
		}
	} else {
		dir = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			relDir, relErr := filepath.Rel(root, dir)
			if relErr == nil && relDir != "." && !strings.HasPrefix(relDir, "..") {
				return filepath.ToSlash(filepath.Join("thoughts", relDir))
			}
		}
		if dir == root || dir == filepath.Dir(dir) {
			break
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func (s *Service) FindSharedThreadForDoc(
	ctx context.Context,
	docPath string,
) (string, error) {
	want := s.thoughtsSharedThreadKey(docPath)
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
		plan := s.thoughtsSharedThreadKey(group.PlanDir)
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

type RootThreadPreview struct {
	ID          string
	Author      string
	Initial     string
	AvatarClass string
	Preview     string
	When        string
	ReplyLabel  string
	Href        string
}

func (s *Service) RenderRootThreadsIndex(
	ctx context.Context,
	userEmail string,
) (templ.Component, bool, error) {
	_ = userEmail
	rows, err := s.listRootThreadPreviews(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	return RootThreadsIndex(rows), true, nil
}

func (s *Service) listRootThreadPreviews(
	ctx context.Context,
) ([]RootThreadPreview, error) {
	if s == nil || s.queries == nil {
		return nil, nil
	}
	listed, err := s.queries.ListSharedAgentThreadsWithWorkspace(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RootThreadPreview, 0, len(listed))
	for _, row := range listed {
		if row.PlanDirRel.Valid && strings.TrimSpace(row.PlanDirRel.String) != "" {
			continue
		}
		if strings.TrimSpace(row.RoomKind) != "" {
			continue
		}
		thread, err := s.queries.GetSharedAgentThread(ctx, row.ID)
		if err != nil {
			continue
		}
		stable, err := s.buildStableTranscript(ctx, thread)
		if err != nil {
			continue
		}
		first, ok := firstUserTranscriptMessage(stable)
		if !ok {
			continue
		}
		author := strings.TrimSpace(first.AuthorName)
		if author == "" {
			author = strings.TrimSpace(thread.UserEmail)
		}
		if author == "" {
			author = strings.TrimSpace(thread.Title)
		}
		initial := strings.TrimSpace(first.AuthorInitial)
		if initial == "" {
			initial = workbenchThreadInitial(author)
		}
		avatar := strings.TrimSpace(first.AvatarBg)
		if avatar == "" {
			avatar = "bg-slate-600"
		}
		replies := countTranscriptReplies(stable)
		replyLabel := ""
		if replies > 0 {
			replyLabel = formatReplyCount(replies)
		}
		out = append(out, RootThreadPreview{
			ID:          thread.ID,
			Author:      author,
			Initial:     initial,
			AvatarClass: avatar,
			Preview:     compactThreadPreview(first.Content),
			When:        formatRootThreadWhen(thread.UpdatedAt),
			ReplyLabel:  replyLabel,
			Href:        threadWorkbenchHref(thread.ID, ""),
		})
	}
	return out, nil
}

func firstUserTranscriptMessage(stable []TranscriptMessage) (TranscriptMessage, bool) {
	for _, msg := range stable {
		if msg.ToolResult || strings.TrimSpace(msg.ToolCallID) != "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		return msg, true
	}
	return TranscriptMessage{}, false
}

func countTranscriptReplies(stable []TranscriptMessage) int {
	n := 0
	for _, msg := range stable {
		if msg.ToolResult || strings.TrimSpace(msg.ToolCallID) != "" {
			continue
		}
		n++
	}
	if n <= 1 {
		return 0
	}
	return n - 1
}

func compactThreadPreview(raw string) string {
	fields := strings.Fields(strings.TrimSpace(raw))
	s := strings.Join(fields, " ")
	runes := []rune(s)
	if len(runes) > 160 {
		return string(runes[:157]) + "…"
	}
	return s
}

func workbenchThreadInitial(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) == 0 {
		return "?"
	}
	return strings.ToUpper(string(runes[0:1]))
}

func formatRootThreadWhen(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	local := at.Local()
	now := time.Now()
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return local.Format("3:04 PM")
	}
	return local.Format("Jan 2")
}

func (s *Service) RenderSharedThreadChat(
	ctx context.Context,
	threadID, userEmail string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "", "")
}

func (s *Service) RenderSharedThreadChatOpen(
	ctx context.Context,
	threadID, userEmail, openParentEntryID string,
) (templ.Component, error) {
	return s.renderSharedThreadChat(ctx, threadID, userEmail, "", openParentEntryID)
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
