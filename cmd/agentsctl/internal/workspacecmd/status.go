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
	fmt.Fprintf(out, "checkout: %s\n", cfg.Metadata.CheckoutPath)
	fmt.Fprintf(out, "manager_url: %s\n", cfg.ManagerURL)
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
