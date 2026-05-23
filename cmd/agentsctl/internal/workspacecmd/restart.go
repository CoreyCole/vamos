package workspacecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func RunRestart(
	ctx context.Context,
	cfg WorkspaceCLIConfig,
	components []string,
	force bool,
	out io.Writer,
) error {
	if strings.TrimSpace(cfg.ManagerURL) == "" ||
		strings.TrimSpace(cfg.RestartToken) == "" {
		return fmt.Errorf(
			"workspace restart requires manager URL and restart token in %s",
			cfg.EnvPath,
		)
	}
	if len(components) == 0 {
		components = []string{"web", "ts_worker"}
	}
	body, err := json.Marshal(struct {
		Slug         string   `json:"slug"`
		CheckoutPath string   `json:"checkout_path"`
		Components   []string `json:"components,omitempty"`
		Force        bool     `json:"force,omitempty"`
	}{
		Slug:         cfg.Metadata.Slug,
		CheckoutPath: cfg.Metadata.CheckoutPath,
		Components:   components,
		Force:        force,
	})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(cfg.ManagerURL, "/") + "/internal/workspaces/restart"
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CN-Agents-Workspace-Restart-Token", cfg.RestartToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"workspace restart API returned %s\n%s",
			resp.Status,
			strings.TrimSpace(string(data)),
		)
	}
	fmt.Fprintf(out, "restart accepted: %s\n", resp.Status)
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 {
		fmt.Fprintln(out, string(trimmed))
	}
	return nil
}

func componentsFromWorkspaceCLIFlags(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	components := make([]string, 0, len(values))
	for _, value := range values {
		switch strings.TrimSpace(value) {
		case "web":
			components = append(components, "web")
		case "ts_worker", "ts-worker":
			components = append(components, "ts_worker")
		default:
			return nil, fmt.Errorf(
				"unknown workspace restart component %q (allowed: web, ts_worker, ts-worker)",
				value,
			)
		}
	}
	return components, nil
}
