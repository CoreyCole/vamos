package agentchat

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/pkg/db"
)

type PlanDirRelBackfillResult struct {
	Scanned    int
	Updated    int
	Unresolved int
}

// resolvePlanDirRel looks up a plan_workspaces row for cwd / absolute / relative
// plan identity. Returns NULL when unresolved (freeform).
func (s *Service) resolvePlanDirRel(ctx context.Context, sources ...string) sql.NullString {
	if s == nil || s.queries == nil {
		return sql.NullString{}
	}
	for _, source := range sources {
		if rel, ok := s.lookupPlanDirRel(ctx, source); ok {
			return sql.NullString{String: rel, Valid: true}
		}
	}
	return sql.NullString{}
}

func (s *Service) lookupPlanDirRel(ctx context.Context, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	for _, candidate := range s.planDirRelLookupCandidates(raw) {
		if row, err := s.queries.GetPlanWorkspace(ctx, candidate); err == nil {
			return row.PlanDirRel, true
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", false
		}
	}
	if abs, ok := s.canonicalPlanDirFromSource(raw); ok {
		if row, err := s.queries.GetPlanWorkspaceByPlanDir(ctx, abs); err == nil {
			return row.PlanDirRel, true
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", false
		}
		rel := s.planSidebarRel(abs)
		if rel != "" {
			if row, err := s.queries.GetPlanWorkspace(ctx, rel); err == nil {
				return row.PlanDirRel, true
			} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", false
			}
		}
	}
	return "", false
}

func (s *Service) planDirRelLookupCandidates(raw string) []string {
	clean := filepath.ToSlash(strings.TrimSpace(raw))
	if clean == "" {
		return nil
	}
	candidates := []string{clean}
	if strings.HasPrefix(clean, "thoughts/") {
		candidates = append(candidates, strings.TrimPrefix(clean, "thoughts/"))
	}
	if idx := strings.LastIndex(clean, "/thoughts/"); idx >= 0 {
		candidates = append(candidates, clean[idx+len("/thoughts/"):])
	}
	if s.thoughtsRoot != "" {
		if rel, err := filepath.Rel(s.thoughtsRoot, filepath.FromSlash(clean)); err == nil &&
			rel != "." && !strings.HasPrefix(rel, "..") {
			candidates = append(candidates, filepath.ToSlash(rel))
		}
	}
	out := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, c := range candidates {
		c = strings.TrimSpace(strings.TrimSuffix(c, "/"))
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func (s *Service) absolutePlanDirFromRel(ctx context.Context, planDirRel string) (string, bool) {
	planDirRel = strings.TrimSpace(planDirRel)
	if planDirRel == "" {
		return "", false
	}
	if row, err := s.queries.GetPlanWorkspace(ctx, planDirRel); err == nil {
		if abs := strings.TrimSpace(row.PlanDir); abs != "" {
			if canonical, ok := s.canonicalPlanDirFromSource(abs); ok {
				return canonical, true
			}
			return filepath.Clean(abs), true
		}
	}
	if s.thoughtsRoot != "" {
		joined := filepath.Join(s.thoughtsRoot, filepath.FromSlash(planDirRel))
		if canonical, ok := s.canonicalPlanDirFromSource(joined); ok {
			return canonical, true
		}
		return filepath.Clean(joined), true
	}
	return "", false
}

// threadPlanDir prefers FK plan_dir_rel, then cwd / workspace-derived canonical path.
func (s *Service) threadPlanDir(
	ctx context.Context,
	planDirRel sql.NullString,
	cwd string,
	workspaceRoot string,
) (planDir string, ok bool) {
	if planDirRel.Valid {
		if abs, found := s.absolutePlanDirFromRel(ctx, planDirRel.String); found {
			return abs, true
		}
	}
	if planDir, ok = s.canonicalPlanDirFromSource(cwd); ok {
		return planDir, true
	}
	return s.canonicalPlanDirFromSource(workspaceRoot)
}

// ListPlanHomeThreads returns FK-linked threads for a plan. Freeform (NULL) excluded.
func (s *Service) ListPlanHomeThreads(
	ctx context.Context,
	planDirRel string,
) ([]db.AgentThread, error) {
	planDirRel = strings.TrimSpace(planDirRel)
	if planDirRel == "" {
		return nil, nil
	}
	return s.queries.ListAgentThreadsByPlanDirRel(
		ctx,
		sql.NullString{String: planDirRel, Valid: true},
	)
}

// MostRecentPlanHomeThread returns the latest FK-linked thread for a plan.
// Falls back to cwd-derived dual-read only when no FK row exists yet (migration).
func (s *Service) MostRecentPlanHomeThread(
	ctx context.Context,
	planDirRel string,
	absolutePlanDir string,
) (db.AgentThread, bool, error) {
	planDirRel = strings.TrimSpace(planDirRel)
	if planDirRel != "" {
		thread, err := s.queries.GetMostRecentAgentThreadByPlanDirRel(
			ctx,
			sql.NullString{String: planDirRel, Valid: true},
		)
		if err == nil {
			return thread, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return db.AgentThread{}, false, err
		}
	}
	absolutePlanDir = strings.TrimSpace(absolutePlanDir)
	if planDirRel == "" && absolutePlanDir == "" {
		return db.AgentThread{}, false, nil
	}
	if planDirRel == "" {
		if rel, ok := s.lookupPlanDirRel(ctx, absolutePlanDir); ok {
			planDirRel = rel
		}
	}
	rows, err := s.queries.ListSharedAgentThreadsByPlanDir(
		ctx,
		db.ListSharedAgentThreadsByPlanDirParams{
			PlanDirRel: sql.NullString{String: planDirRel, Valid: planDirRel != ""},
			PlanDir:    sql.NullString{String: absolutePlanDir, Valid: absolutePlanDir != ""},
		},
	)
	if err != nil {
		return db.AgentThread{}, false, err
	}
	if len(rows) == 0 {
		return db.AgentThread{}, false, nil
	}
	// Prefer an already-linked row if dual-read returned mixed results.
	for _, row := range rows {
		if row.PlanDirRel.Valid && row.PlanDirRel.String == planDirRel {
			return row, true, nil
		}
	}
	// Migration fallback: only surface NULL threads that still resolve to this plan.
	wantAbs := absolutePlanDir
	if wantAbs == "" {
		if abs, ok := s.absolutePlanDirFromRel(ctx, planDirRel); ok {
			wantAbs = abs
		}
	}
	for _, row := range rows {
		if row.PlanDirRel.Valid {
			continue
		}
		if wantAbs == "" {
			continue
		}
		got, ok := s.canonicalPlanDirFromSource(row.Cwd)
		if ok && sameFilesystemPath(got, wantAbs) {
			return row, true, nil
		}
	}
	return db.AgentThread{}, false, nil
}

// BackfillAgentThreadPlanDirRels maps unresolved threads onto plan_workspaces
// via cwd / agent_sessions.plan_dir. Leaves freeform NULL when no plan row exists.
func (s *Service) BackfillAgentThreadPlanDirRels(
	ctx context.Context,
) (PlanDirRelBackfillResult, error) {
	result := PlanDirRelBackfillResult{}
	if s == nil || s.queries == nil {
		return result, nil
	}
	rows, err := s.queries.ListAgentThreadsForPlanDirBackfill(ctx)
	if err != nil {
		return result, err
	}
	seen := map[string]struct{}{}
	for _, row := range rows {
		if _, ok := seen[row.ID]; ok {
			continue
		}
		seen[row.ID] = struct{}{}
		result.Scanned++
		rel := s.resolvePlanDirRel(ctx, row.Cwd, nullStringValue(row.SessionPlanDir))
		if !rel.Valid {
			result.Unresolved++
			continue
		}
		if err := s.queries.SetAgentThreadPlanDirRel(
			ctx,
			db.SetAgentThreadPlanDirRelParams{
				PlanDirRel: rel,
				ID:         row.ID,
			},
		); err != nil {
			return result, err
		}
		result.Updated++
	}
	return result, nil
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

// attachPlanDirRel fills CreateAgentThreadParams.PlanDirRel when resolvable.
func (s *Service) attachPlanDirRel(
	ctx context.Context,
	params db.CreateAgentThreadParams,
) db.CreateAgentThreadParams {
	if params.PlanDirRel.Valid {
		return params
	}
	params.PlanDirRel = s.resolvePlanDirRel(ctx, params.Cwd)
	return params
}
