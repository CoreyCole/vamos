package markdown

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/CoreyCole/vamos/pkg/agents/roster"
	"github.com/CoreyCole/vamos/pkg/db"
)

func (s *Service) resolvePairwiseThread(
	ctx context.Context,
	slugA, slugB, userEmail string,
) (string, error) {
	if s.queries == nil || s.roster == nil {
		return "", nil
	}
	left, right, err := canonicalPairSlugs(slugA, slugB)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if _, err := s.roster.Get(left); err != nil {
		if errors.Is(err, roster.ErrNotFound) || errors.Is(err, roster.ErrArchived) {
			return "", echo.NewHTTPError(http.StatusNotFound, "agent not found")
		}
		return "", err
	}
	if _, err := s.roster.Get(right); err != nil {
		if errors.Is(err, roster.ErrNotFound) || errors.Is(err, roster.ErrArchived) {
			return "", echo.NewHTTPError(http.StatusNotFound, "agent not found")
		}
		return "", err
	}
	existing, err := s.queries.GetPairwiseThread(ctx, db.GetPairwiseThreadParams{
		PairAgentSlugA: sql.NullString{String: left, Valid: true},
		PairAgentSlugB: sql.NullString{String: right, Valid: true},
	})
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if strings.TrimSpace(userEmail) == "" {
		userEmail = "shared"
	}
	cwd, err := seedPairwiseTree(s.basePath, left, right)
	if err != nil {
		return "", err
	}
	threadID := uuid.NewString()
	if _, err := s.queries.CreateAgentThread(ctx, db.CreateAgentThreadParams{
		ID:        threadID,
		UserEmail: userEmail,
		Title:     left + " / " + right,
		Cwd:       cwd,
		LineageID: uuid.NewString(),
		ProjectID: "",
	}); err != nil {
		return "", err
	}
	if err := s.queries.BindAgentThreadPairwise(ctx, db.BindAgentThreadPairwiseParams{
		PairAgentSlugA: sql.NullString{String: left, Valid: true},
		PairAgentSlugB: sql.NullString{String: right, Valid: true},
		Cwd:            cwd,
		Title:          left + " / " + right,
		ID:             threadID,
	}); err != nil {
		return "", err
	}
	return threadID, nil
}

func canonicalPairSlugs(a, b string) (string, string, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return "", "", fmt.Errorf("pairwise slugs are required")
	}
	if a == b {
		return "", "", fmt.Errorf("pairwise slugs must differ")
	}
	if a < b {
		return a, b, nil
	}
	return b, a, nil
}

func seedPairwiseTree(thoughtsRoot, a, b string) (string, error) {
	left, right, err := canonicalPairSlugs(a, b)
	if err != nil {
		return "", err
	}
	cwd := filepath.Join(thoughtsRoot, "a2a", left+"__"+right)
	for _, dir := range []string{
		filepath.Join(cwd, "sessions", "history"),
		filepath.Join(cwd, "sessions", "handoffs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	current := filepath.Join(cwd, "sessions", "current.jsonl")
	if _, err := os.Stat(current); os.IsNotExist(err) {
		if err := os.WriteFile(current, nil, 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return cwd, nil
}
