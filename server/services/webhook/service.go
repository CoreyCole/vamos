package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/pkg/git"
)

const (
	gitStatusTimeout       = 10 * time.Second
	thoughtsSyncTimeout    = 5 * time.Minute
	rebuildTimeout         = 10 * time.Minute
	gitStatusPathOffsetLen = 3
)

// Service handles GitHub webhook events
type Service struct {
	secret       string
	defaultRoute RepoRoute
	routes       map[string]RepoRoute
}

type RepoRoute struct {
	GitHubRepo    string
	RepoPath      string
	RebuildScript string
	SyncThoughts  bool
}

// NewService creates a new webhook service.
func NewService(secret, repoPath, rebuildScript string) *Service {
	return NewServiceWithRoutes(secret, RepoRoute{
		RepoPath:      repoPath,
		RebuildScript: rebuildScript,
		SyncThoughts:  true,
	}, nil)
}

func NewServiceWithRoutes(secret string, defaultRoute RepoRoute, routes []RepoRoute) *Service {
	s := &Service{
		secret:       secret,
		defaultRoute: defaultRoute,
		routes:       map[string]RepoRoute{},
	}
	for _, route := range routes {
		key := strings.ToLower(strings.TrimSpace(route.GitHubRepo))
		if key == "" {
			continue
		}
		if route.RepoPath == "" {
			route.RepoPath = defaultRoute.RepoPath
		}
		if route.RebuildScript == "" {
			route.RebuildScript = defaultRoute.RebuildScript
		}
		s.routes[key] = route
	}
	return s
}

// PushEvent represents the relevant fields from a GitHub push webhook payload
type PushEvent struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"` //nolint:tagliatelle // GitHub payload uses snake_case.
	} `json:"repository"`
	Pusher struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"pusher"`
}

// HandlePush processes a GitHub push webhook event
func (s *Service) HandlePush(ctx context.Context, payload []byte) error {
	var event PushEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("failed to parse push event: %w", err)
	}

	route, ok := s.routeFor(event.Repository.FullName)
	if !ok {
		s.logEvent("webhook_repo_ignored", map[string]any{
			"ref":        event.Ref,
			"repository": event.Repository.FullName,
		})
		return nil
	}

	s.logEvent("webhook_received", map[string]any{
		"ref":        event.Ref,
		"before":     event.Before,
		"after":      event.After,
		"repository": event.Repository.FullName,
		"repo_path":  route.RepoPath,
		"pusher":     event.Pusher.Name,
	})

	// Capture HEAD before any local thoughts sync. The sync step may pull/rebase
	// while publishing local thoughts; comparing against this original commit keeps
	// code changes from that pull eligible for rebuild.
	beforeCommit, err := git.GetCurrentCommit(ctx, route.RepoPath)
	if err != nil {
		s.logEvent("git_error", map[string]any{
			"operation": "get_commit_before",
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to get current commit: %w", err)
	}

	// Publish local thoughts before refusing a dirty tree. Thought artifacts are
	// expected to accumulate locally; sync-thoughts formats, commits, rebases, and
	// pushes them so webhook deploys are not blocked by thoughts-only changes.
	if route.SyncThoughts {
		if err := s.runThoughtsSync(ctx, route); err != nil {
			s.logEvent("thoughts_sync_error", map[string]any{
				"error": err.Error(),
			})
		}
	}

	// Never stash local changes from the webhook path. This repository often has
	// agent/user work in progress, and hiding it in git stash makes the deploy
	// side effect hard to notice. Refuse to pull instead after thoughts had a
	// chance to sync cleanly.
	if dirty, err := s.getDirtyFiles(ctx, route); err != nil {
		s.logEvent("git_error", map[string]any{
			"operation": "status",
			"error":     err.Error(),
		})
	} else if len(dirty) > 0 {
		s.logEvent("dirty_worktree", map[string]any{
			"files": dirty,
		})
		return fmt.Errorf(
			"repository has local changes; refusing webhook pull: %s",
			strings.Join(dirty, ", "),
		)
	}

	// Pull latest changes
	output, err := git.Pull(ctx, route.RepoPath)
	if err != nil {
		s.logEvent("git_error", map[string]any{
			"operation": "pull",
			"error":     err.Error(),
			"output":    output,
		})
		return fmt.Errorf("git pull failed: %w", err)
	}

	s.logEvent("git_pull_success", map[string]any{
		"output": output,
	})

	// Get new HEAD after pull
	afterCommit, err := git.GetCurrentCommit(ctx, route.RepoPath)
	if err != nil {
		s.logEvent("git_error", map[string]any{
			"operation": "get_commit_after",
			"error":     err.Error(),
		})
		return fmt.Errorf("failed to get new commit: %w", err)
	}

	// Check if anything changed
	if beforeCommit == afterCommit {
		s.logEvent("no_changes", map[string]any{
			"commit": beforeCommit,
		})
		return nil
	}

	// Get list of changed files
	changedFiles, err := git.GetChangedFiles(ctx, route.RepoPath, beforeCommit, afterCommit)
	if err != nil {
		s.logEvent("git_error", map[string]any{
			"operation": "get_changed_files",
			"error":     err.Error(),
		})
		// Continue anyway - we'll rebuild to be safe
	}

	s.logEvent("files_changed", map[string]any{
		"before":        beforeCommit,
		"after":         afterCommit,
		"changed_count": len(changedFiles),
		"files":         changedFiles,
	})

	// Check if any non-thoughts files changed (code changes requiring rebuild)
	needsRebuild := s.hasCodeChanges(changedFiles)

	if needsRebuild {
		s.logEvent("rebuild_triggered", map[string]any{
			"script": route.RebuildScript,
		})

		// Run rebuild in background so we can return HTTP response immediately
		// (nix develop can take a long time)
		go func() {
			if err := s.runRebuildScript(route); err != nil {
				s.logEvent("rebuild_error", map[string]any{
					"error": err.Error(),
				})
				return
			}
			s.logEvent("rebuild_success", nil)
		}()
	} else {
		s.logEvent("rebuild_skipped", map[string]any{
			"reason": "only thoughts files changed",
		})
	}

	return nil
}

func (s *Service) routeFor(fullName string) (RepoRoute, bool) {
	key := strings.ToLower(strings.TrimSpace(fullName))
	if len(s.routes) == 0 {
		return s.defaultRoute, true
	}
	route, ok := s.routes[key]
	return route, ok
}

func (s *Service) runThoughtsSync(ctx context.Context, route RepoRoute) error {
	script := filepath.Join(route.RepoPath, "scripts", "sync-thoughts.sh")
	if _, err := os.Stat(script); err != nil {
		if os.IsNotExist(err) {
			s.logEvent("thoughts_sync_skipped", map[string]any{
				"reason": "script_missing",
				"script": script,
			})
			return nil
		}
		return fmt.Errorf("stat sync-thoughts script: %w", err)
	}

	syncCtx, cancel := context.WithTimeout(ctx, thoughtsSyncTimeout)
	defer cancel()

	//nolint:gosec // script path is derived from configured repoPath, not request input.
	cmd := exec.CommandContext(syncCtx, "/bin/bash", script)
	cmd.Dir = route.RepoPath
	output, err := cmd.CombinedOutput()
	s.logEvent("thoughts_sync_output", map[string]any{
		"output": string(output),
	})
	if err != nil {
		return fmt.Errorf("sync thoughts failed: %w", err)
	}
	return nil
}

func (s *Service) getDirtyFiles(ctx context.Context, route RepoRoute) ([]string, error) {
	statusCtx, cancel := context.WithTimeout(ctx, gitStatusTimeout)
	defer cancel()

	//nolint:gosec // repoPath is configured by the server owner, not request input.
	cmd := exec.CommandContext(
		statusCtx,
		git.Binary(),
		"-C",
		route.RepoPath,
		"status",
		"--porcelain",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) > gitStatusPathOffsetLen {
			files = append(files, strings.TrimSpace(line[gitStatusPathOffsetLen:]))
		}
	}
	return files, nil
}

// hasCodeChanges returns true if any non-thoughts files were changed
func (s *Service) hasCodeChanges(files []string) bool {
	for _, file := range files {
		if !strings.HasPrefix(file, "thoughts/") {
			return true
		}
	}
	return false
}

// runRebuildScript executes the rebuild script
func (s *Service) runRebuildScript(route RepoRoute) error {
	ctx, cancel := context.WithTimeout(context.Background(), rebuildTimeout)
	defer cancel()

	//nolint:gosec // rebuildScript is configured by the server owner, not request input.
	cmd := exec.CommandContext(ctx, "/bin/bash", route.RebuildScript)
	cmd.Dir = route.RepoPath
	output, err := cmd.CombinedOutput()

	s.logEvent("rebuild_output", map[string]any{
		"output": string(output),
	})

	return err
}

// logEvent logs a webhook event in NDJSON format to stderr
func (s *Service) logEvent(event string, data map[string]any) {
	logEntry := map[string]any{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"event": event,
	}
	for k, v := range data {
		logEntry[k] = v
	}

	if jsonBytes, err := json.Marshal(logEntry); err == nil {
		fmt.Fprintln(os.Stderr, string(jsonBytes))
	}
}
