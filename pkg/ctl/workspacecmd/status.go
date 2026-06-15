package workspacecmd

import (
	"context"
	"fmt"
	"io"
	"sort"
)

func RunStatus(ctx context.Context, cfg WorkspaceCLIConfig, out io.Writer) error {
	_ = ctx
	fmt.Fprintf(out, "slug: %s\n", cfg.Metadata.Slug)
	if cfg.Metadata.ProjectID != "" {
		fmt.Fprintf(out, "project_id: %s\n", cfg.Metadata.ProjectID)
	}
	fmt.Fprintf(out, "checkout: %s\n", cfg.Metadata.CheckoutPath)
	fmt.Fprintf(out, "manager_url: %s\n", cfg.ManagerURL)
	fmt.Fprintln(out, "manager_lifecycle: unavailable from local-only command")
	fmt.Fprintln(out, "manager_lifecycle_hint: use just build from a managed checkout or the manager Workspaces page for source-labeled lifecycle and scheduled sync diagnostics")
	fmt.Fprintln(out, "local_runtime_diagnostics: source .vamos/run/status.json; diagnostic only")
	fmt.Fprintf(out, "status: %s\n", cfg.Status.Status)
	if cfg.Status.Phase != "" {
		fmt.Fprintf(out, "phase: %s\n", cfg.Status.Phase)
	}
	if cfg.Status.Error != "" {
		fmt.Fprintf(out, "error: %s\n", cfg.Status.Error)
	}
	printStringMap(out, "logs", cfg.Status.Logs)
	printIntMap(out, "ports", cfg.Status.Ports)
	printIntMap(out, "pids", cfg.Status.PIDs)
	return nil
}

func printStringMap(out io.Writer, label string, values map[string]string) {
	if len(values) == 0 {
		return
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(out, "%s.%s: %s\n", label, key, values[key])
	}
}

func printIntMap(out io.Writer, label string, values map[string]int) {
	if len(values) == 0 {
		return
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(out, "%s.%s: %d\n", label, key, values[key])
	}
}
