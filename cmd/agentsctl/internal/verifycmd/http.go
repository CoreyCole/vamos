package verifycmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

const (
	httpsProbeTimeout = 15 * time.Second
	probeBodyLimit    = 64 * 1024
	errorBodyLimit    = 4096
	pollInterval      = 250 * time.Millisecond
)

func ResolveHost(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func ProbeHTTPS(ctx context.Context, rawURL string, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: httpsProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Fprintf(out, "%s %s\n", resp.Proto, resp.Status)
	for k, vals := range resp.Header {
		fmt.Fprintf(out, "%s: %s\n", k, strings.Join(vals, ", "))
	}
	fmt.Fprintln(out)
	_, _ = io.Copy(out, io.LimitReader(resp.Body, probeBodyLimit))
	if resp.StatusCode < 200 || resp.StatusCode >= 500 {
		return fmt.Errorf("%s returned HTTP %d", rawURL, resp.StatusCode)
	}
	return nil
}

func ProbeHostPreservation(
	ctx context.Context,
	baseURL, childURL string,
	out io.Writer,
) error {
	fmt.Fprintf(out, "manager: %s\n", baseURL)
	if err := ProbeHTTPS(ctx, baseURL, out); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nchild: %s\n", childURL)
	return ProbeHTTPS(ctx, childURL, out)
}

func StartServerVerifyRun(
	ctx context.Context,
	cfg WorkspaceVerifyConfig,
	req workspaces.VerifyWorkspaceRequest,
) (workspaces.VerifyWorkspaceRun, error) {
	var run workspaces.VerifyWorkspaceRun
	endpoint, err := joinURL(cfg.BaseURL, "/internal/workspaces/verify")
	if err != nil {
		return run, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return run, err
	}
	hreq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return run, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("X-CN-Agents-Workspace-Restart-Token", cfg.RestartToken)
	resp, err := http.DefaultClient.Do(hreq)
	if err != nil {
		return run, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return run, fmt.Errorf(
			"start server verify returned HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(b)),
		)
	}
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return run, err
	}
	return run, nil
}

func WaitServerVerifyRun(
	ctx context.Context,
	cfg WorkspaceVerifyConfig,
	runID string,
) (workspaces.VerifyWorkspaceRun, error) {
	var run workspaces.VerifyWorkspaceRun
	endpoint, err := joinURL(
		cfg.BaseURL,
		"/internal/workspaces/verify/"+url.PathEscape(runID),
	)
	if err != nil {
		return run, err
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		hreq, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			endpoint,
			http.NoBody,
		)
		if err != nil {
			return run, err
		}
		hreq.Header.Set("X-CN-Agents-Workspace-Restart-Token", cfg.RestartToken)
		resp, err := http.DefaultClient.Do(hreq)
		if err != nil {
			return run, err
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
			_ = resp.Body.Close()
			return run, fmt.Errorf(
				"get server verify returned HTTP %d: %s",
				resp.StatusCode,
				strings.TrimSpace(string(b)),
			)
		}
		if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
			_ = resp.Body.Close()
			return run, err
		}
		_ = resp.Body.Close()
		if run.Status == workspaces.VerifyRunPassed {
			return run, nil
		}
		if run.Status == workspaces.VerifyRunFailed {
			if run.Error != nil {
				return run, errors.New(run.Error.Message)
			}
			return run, errors.New("server verification failed")
		}
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-ticker.C:
		}
	}
}

func RunServerLifecyclePhase(
	ctx context.Context,
	cfg WorkspaceVerifyConfig,
	req workspaces.VerifyWorkspaceRequest,
) (workspaces.VerifyWorkspaceRun, error) {
	run, err := StartServerVerifyRun(ctx, cfg, req)
	if err != nil {
		return run, err
	}
	return WaitServerVerifyRun(ctx, cfg, run.ID)
}

func joinURL(base, path string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
