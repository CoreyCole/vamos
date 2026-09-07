package agentchat

import (
	"context"
	"strings"
)

// ListSharedThreadIDs returns shared thread IDs in workbench list order for
// AI-470 room→thread fixture mapping (dm/bot, dm/research, group, …).
func (s *Service) ListSharedThreadIDs(ctx context.Context) ([]string, error) {
	groups, err := s.ListWorkbenchThreads(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	seen := map[string]bool{}
	for _, group := range groups {
		for _, thread := range group.Threads {
			id := strings.TrimSpace(thread.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}
