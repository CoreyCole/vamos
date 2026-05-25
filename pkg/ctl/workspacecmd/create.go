package workspacecmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

type CreateOptions struct {
	PlanPath         string
	ManagerURL       string
	RestartToken     string
	WorkspaceSlug    string
	RequestedPath    string
	SourceCheckout   string
	BaselineCheckout string
	TrunkBranch      string
	ParentStackRef   string
	ReviewFollowup   bool
	Force            bool
}

func RunCreate(ctx context.Context, opts CreateOptions, out io.Writer) error {
	if strings.TrimSpace(opts.PlanPath) == "" {
		return fmt.Errorf("workspace create requires --plan")
	}
	if strings.TrimSpace(opts.ManagerURL) == "" || strings.TrimSpace(opts.RestartToken) == "" {
		return fmt.Errorf("workspace create requires --manager-url and --restart-token")
	}
	planPath := filepath.Clean(opts.PlanPath)
	input := workspaces.WorkspaceProvisionInput{
		PlanPath:         planPath,
		PlanDir:          filepath.Dir(planPath),
		WorkspaceSlug:    strings.TrimSpace(opts.WorkspaceSlug),
		RequestedPath:    strings.TrimSpace(opts.RequestedPath),
		SourceCheckout:   strings.TrimSpace(opts.SourceCheckout),
		BaselineCheckout: strings.TrimSpace(opts.BaselineCheckout),
		TrunkBranch:      strings.TrimSpace(opts.TrunkBranch),
		ParentStackRef:   strings.TrimSpace(opts.ParentStackRef),
		ReviewFollowup:   opts.ReviewFollowup,
		Force:            opts.Force,
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(opts.ManagerURL, "/") + "/internal/workspaces/provision"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Vamos-Workspace-Restart-Token", opts.RestartToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("workspace provision API returned %s\n%s", resp.Status, strings.TrimSpace(string(data)))
	}
	if _, err := out.Write(bytes.TrimSpace(data)); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out)
	if workspaceURL := workspaceURLFromResponse(data, opts.WorkspaceSlug, opts.ManagerURL); workspaceURL != "" {
		_, _ = fmt.Fprintf(out, "workspace URL: %s\n", workspaceURL)
	}
	return nil
}
