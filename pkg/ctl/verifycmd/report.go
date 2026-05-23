package verifycmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoreyCole/vamos/server/services/workspaces"
)

func WriteWorkspaceVerifyReport(report WorkspaceVerifyReport, dir string) error {
	if dir == "" {
		dir = report.Artifacts["report_dir"]
	}
	if dir == "" {
		dir = filepath.Join("tmp", "workspace-verification")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if report.Artifacts == nil {
		report.Artifacts = map[string]string{}
	}
	report.Artifacts["report_dir"] = dir
	if _, ok := report.Artifacts["manager-log-tail"]; !ok {
		path := filepath.Join(dir, "manager-log-tail.txt")
		_ = os.WriteFile(path, []byte("manager log unavailable from agentsctl\n"), 0o600)
		report.Artifacts["manager-log-tail"] = path
	}
	if len(report.ServerRuns) > 0 {
		if tail := report.ServerRuns[len(report.ServerRuns)-1].Diagnostics.LogTail; strings.TrimSpace(
			tail,
		) != "" {
			path := filepath.Join(dir, "child-log-tail.txt")
			_ = os.WriteFile(path, []byte(tail), 0o600)
			report.Artifacts["child-log-tail"] = path
		}
	}
	if err := writeJSON(filepath.Join(dir, "summary.json"), report); err != nil {
		return err
	}
	if err := writeJSON(
		filepath.Join(dir, "server-runs.json"),
		report.ServerRuns,
	); err != nil {
		return err
	}
	return os.WriteFile(
		filepath.Join(dir, "summary.md"),
		[]byte(markdownSummary(report)),
		0o600,
	)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func markdownSummary(report WorkspaceVerifyReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Workspace Verification: %s\n\n", report.Summary.Slug)
	fmt.Fprintf(&b, "Status: `%s`\n\n", report.Summary.Status)
	fmt.Fprintf(
		&b,
		"```workspace-verification\nslug: %s\nstatus: %s\nscope: %s\n```\n\n",
		report.Summary.Slug,
		report.Summary.Status,
		report.Summary.Scope,
	)
	if report.Failure != nil {
		fmt.Fprintf(
			&b,
			"Failure: `%s` %s\n\n",
			report.Failure.Layer,
			report.Failure.Message,
		)
	}
	layers := make([]string, 0, len(report.Summary.Layers))
	for layer := range report.Summary.Layers {
		layers = append(layers, string(layer))
	}
	sort.Strings(layers)
	b.WriteString("## Layers\n\n")
	for _, layer := range layers {
		fmt.Fprintf(
			&b,
			"- `%s`: %s\n",
			layer,
			report.Summary.Layers[workspaces.VerificationLayer(layer)],
		)
	}
	b.WriteString("\n## Client steps\n\n")
	for _, step := range report.ClientSteps {
		fmt.Fprintf(&b, "- `%s` (%s): %s", step.Name, step.Layer, step.Status)
		if step.Error != "" {
			fmt.Fprintf(&b, " — %s", step.Error)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n## Server runs\n\n")
	for _, run := range report.ServerRuns {
		fmt.Fprintf(&b, "- `%s`: %s\n", run.ID, run.Status)
	}
	return b.String()
}
