package agentchat

import (
	"context"
	"sort"
	"strings"
	"time"
)

// HermesThread is the shared, disk-backed conversation presentation model.
// Ownership controls prompting only; it never controls inspection.
type HermesThread struct {
	ID          string
	OwnerEmail  string
	Title       string
	WorkspaceID string
	PlanDir     string
	UpdatedAt   time.Time
}

type ThreadQuery struct {
	PlanDir string
	Search  string
}

type HermesThreadGroup struct {
	PlanDir string
	Threads []HermesThread
}

// CanPromptThread intentionally does not gate visibility. Plan-owned threads
// are shared organizational artifacts; only delivery to Hermes is owner-only.
func (s *Service) CanPromptThread(userEmail string, thread HermesThread) bool {
	return strings.EqualFold(
		strings.TrimSpace(userEmail),
		strings.TrimSpace(thread.OwnerEmail),
	)
}

// ListHermesThreads scans durable transcript artifacts so a new thread is
// available before the disposable projection has refreshed. Metadata not yet
// projected is represented by its stable transcript ID.
func (s *Service) ListHermesThreads(
	ctx context.Context,
	query ThreadQuery,
) ([]HermesThread, error) {
	_ = ctx
	root := strings.TrimSpace(s.thoughtsRoot)
	plan := strings.TrimSpace(query.PlanDir)
	if plan == "" {
		plan = root
	}
	artifacts, err := ScanHermesThreads(root, plan)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query.Search))
	threads := make([]HermesThread, 0, len(artifacts))
	for _, item := range artifacts {
		t := HermesThread{
			ID:        item.ID,
			Title:     item.ID,
			PlanDir:   item.PlanDir,
			UpdatedAt: item.UpdatedAt,
		}
		if needle != "" && !strings.Contains(strings.ToLower(t.Title), needle) {
			continue
		}
		threads = append(threads, t)
	}
	sort.SliceStable(
		threads,
		func(i, j int) bool { return threads[i].UpdatedAt.After(threads[j].UpdatedAt) },
	)
	return threads, nil
}

func GroupHermesThreads(threads []HermesThread) []HermesThreadGroup {
	byPlan := map[string][]HermesThread{}
	for _, thread := range threads {
		byPlan[thread.PlanDir] = append(byPlan[thread.PlanDir], thread)
	}
	groups := make([]HermesThreadGroup, 0, len(byPlan))
	for plan, items := range byPlan {
		groups = append(groups, HermesThreadGroup{PlanDir: plan, Threads: items})
	}
	sort.Slice(
		groups,
		func(i, j int) bool { return groups[i].PlanDir < groups[j].PlanDir },
	)
	return groups
}

type HermesThreadsPanelArgs struct {
	UserEmail    string
	CurrentFile  string
	PlanDir      string
	Threads      []HermesThread
	SelectedID   string
	SearchAction string
}
