package agentchat

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
)

// SessionProjectionRebuildResult describes a best-effort rebuild of the
// disposable session index. Durable transcript and Pi session files remain the
// source of truth; this only refreshes their searchable pointers.
type SessionProjectionRebuildResult struct {
	Plans    int
	Sessions int
	Changed  bool
}

// RebuildSessionProjection scans plan directories from disk and recreates the
// plan-owned session index. It intentionally has no dependency on workflow
// state, active runs, or a live Hermes process.
func (s *Service) RebuildSessionProjection(
	ctx context.Context,
) (SessionProjectionRebuildResult, error) {
	root := s.thoughtsRoot
	result := SessionProjectionRebuildResult{}
	syncer := &PlanWorkspaceSyncer{
		Queries: s.queries,
		Scanner: PlanWorkspaceScanner{ThoughtsRoot: root},
	}
	err := filepath.WalkDir(
		root,
		func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || entry.Name() != "AGENTS.md" {
				return nil
			}
			count, changed, err := syncer.syncPlanAgentSessions(ctx, filepath.Dir(path))
			if err != nil {
				return err
			}
			result.Plans++
			result.Sessions += count
			result.Changed = result.Changed || changed
			return nil
		},
	)
	if os.IsNotExist(err) {
		return result, nil
	}
	return result, err
}
