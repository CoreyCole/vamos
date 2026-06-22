package qrspicmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ShellCommandRunner struct{}

func (ShellCommandRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
	}
	return result, err
}

func commandRunner(d deps) CommandRunner {
	if d.CommandRunner != nil {
		return d.CommandRunner
	}
	return ShellCommandRunner{}
}

func CheckPiCompatibility(ctx context.Context, req PiCompatibilityRequest, runner CommandRunner) (PiCompatibilityReport, error) {
	if runner == nil {
		runner = ShellCommandRunner{}
	}
	binary := strings.TrimSpace(req.PiBinary)
	if binary == "" {
		binary = "pi"
	}
	report := PiCompatibilityReport{OK: true, PiBinary: binary}
	help, err := runner.Run(ctx, binary, "--help")
	text := help.Stdout + "\n" + help.Stderr
	if line := firstLine(strings.TrimSpace(text)); line != "" {
		report.Evidence = append(report.Evidence, line)
	}
	if err != nil {
		report.OK = false
		report.Problems = append(report.Problems, PreflightProblem{Kind: "pi_help_failed", Severity: "error", Summary: "pi --help failed", Evidence: strings.TrimSpace(help.Stderr)})
		return report, nil
	}
	for _, flag := range []string{"--session-id", "--session-dir", "--name"} {
		if !strings.Contains(text, flag) {
			report.OK = false
			report.Problems = append(report.Problems, PreflightProblem{Kind: "pi_flag_missing", Severity: "error", Summary: "Pi CLI missing required q-manager flag", Evidence: flag})
		}
	}
	if req.UsesExtension && !strings.Contains(text, "--extension") {
		report.OK = false
		report.Problems = append(report.Problems, PreflightProblem{Kind: "pi_flag_missing", Severity: "error", Summary: "Pi CLI missing extension flag", Evidence: "--extension"})
	}
	if version, err := runner.Run(ctx, binary, "--version"); err == nil {
		report.Version = firstLine(strings.TrimSpace(version.Stdout + "\n" + version.Stderr))
	}
	return report, nil
}

func CheckStateRootWritable(path string) StateRootReport {
	report := StateRootReport{Path: path, OK: true, Writable: true}
	if strings.TrimSpace(path) == "" {
		report.OK = false
		report.Writable = false
		report.Evidence = append(report.Evidence, "state root path empty")
		return report
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		report.OK = false
		report.Writable = false
		report.Evidence = append(report.Evidence, err.Error())
		return report
	}
	probe := filepath.Join(path, ".qrspi-preflight-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		report.OK = false
		report.Writable = false
		report.Evidence = append(report.Evidence, err.Error())
		return report
	}
	_ = os.Remove(probe)
	report.Evidence = append(report.Evidence, "state root writable")
	return report
}

func CheckTmuxAvailable(ctx context.Context, tmux TmuxClient, managerPane string) TmuxHealthReport {
	report := TmuxHealthReport{OK: true, PaneID: managerPane}
	if tmux == nil {
		tmux = ShellTmuxClient{}
	}
	if strings.TrimSpace(managerPane) == "" {
		report.Evidence = append(report.Evidence, "manager pane not configured; skipping pane existence check")
		return report
	}
	ok, err := tmux.PaneExists(ctx, TmuxPane{ID: managerPane})
	if err != nil || !ok {
		report.OK = false
		report.Evidence = append(report.Evidence, fmt.Sprintf("manager pane unavailable: %v", err))
		return report
	}
	report.Evidence = append(report.Evidence, "manager pane available")
	return report
}

func CheckQRSPIPreflight(ctx context.Context, state ManagerState, opts PreflightOptions, d deps) (DoctorReport, error) {
	stateRootPath := opts.StateRootPath
	if stateRootPath == "" {
		root, err := stateRoot(d)
		if err != nil {
			return DoctorReport{}, err
		}
		stateRootPath = root
	}
	pi, err := CheckPiCompatibility(ctx, PiCompatibilityRequest{PiBinary: opts.PiBinary, UsesExtension: opts.UsesExtension}, commandRunner(d))
	if err != nil {
		return DoctorReport{}, err
	}
	report := DoctorReport{
		StateFile:   opts.StateFile,
		Pi:          pi,
		Tmux:        CheckTmuxAvailable(ctx, tmuxClient(d), firstNonEmpty(opts.ManagerPaneID, state.ManagerPaneID)),
		StateRoot:   CheckStateRootWritable(stateRootPath),
		SafeCommand: continueCommand(opts.StateFile),
	}
	if state.ActiveChild != nil {
		if health, _ := InspectActiveChildHealth(ctx, state, opts.StateFile, d); health.Status != "" {
			report.ActiveChild = &health
		}
		if status, _ := ReadChildStatus(state.ActiveChild.StatusPath); status != nil {
			report.LatestStatus = status
		}
	}
	return report, nil
}

func RunDoctor(ctx context.Context, opts DoctorOptions, d deps, out io.Writer) error {
	out = ensureWriter(out)
	var state ManagerState
	if strings.TrimSpace(opts.StateFile) != "" {
		loaded, err := stateStore(d, "", time.Now).Load(opts.StateFile)
		if err != nil {
			return err
		}
		state = loaded
	}
	report, err := CheckQRSPIPreflight(ctx, state, PreflightOptions{StateFile: opts.StateFile, ManagerPaneID: state.ManagerPaneID}, d)
	if err != nil {
		return err
	}
	if strings.EqualFold(opts.Output, "json") || strings.EqualFold(opts.Output, "ndjson") {
		return json.NewEncoder(out).Encode(report)
	}
	writeDoctorText(out, report)
	return nil
}

func writeDoctorText(out io.Writer, report DoctorReport) {
	fmt.Fprintf(out, "pi: %s\n", okText(report.Pi.OK))
	if report.Pi.Version != "" {
		fmt.Fprintf(out, "pi version: %s\n", report.Pi.Version)
	}
	for _, problem := range report.Pi.Problems {
		fmt.Fprintf(out, "pi problem: %s: %s\n", problem.Summary, problem.Evidence)
	}
	fmt.Fprintf(out, "state root: %s\n", okText(report.StateRoot.OK))
	fmt.Fprintf(out, "tmux: %s\n", okText(report.Tmux.OK))
	if report.ActiveChild != nil {
		fmt.Fprintf(out, "active child health: %s\n", report.ActiveChild.Status)
	}
	if report.LatestStatus != nil {
		fmt.Fprintf(out, "latest status: exitCode=%d\n", report.LatestStatus.ExitCode)
	}
	fmt.Fprintf(out, "safe command: %s\n", report.SafeCommand)
}

func okText(ok bool) string {
	if ok {
		return "ok"
	}
	return "failed"
}

func firstLine(text string) string {
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func ReadChildStatus(path string) (*ChildStatus, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var status ChildStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func BuildPreflightFailedCard(report PiCompatibilityReport, stateFile string) *ManagerActionCard {
	if report.OK {
		return nil
	}
	evidence := []string{"pi: " + report.PiBinary}
	for _, problem := range report.Problems {
		evidence = append(evidence, strings.TrimSpace(problem.Summary+": "+problem.Evidence))
	}
	return &ManagerActionCard{Kind: ActionPiCompatibilityFailed, Severity: "error", Summary: "Pi CLI incompatible with q-manager child launch", Evidence: evidence, RecommendedAction: "update Pi or adjust q-manager Pi command contract", SafeCommand: fmt.Sprintf("vamos qrspi doctor --state-file %s", stateFile), ContinueCommand: continueCommand(stateFile), RequiresHuman: false}
}

func BuildChildLaunchFailedCard(health ActiveChildHealth, state ManagerState, stateFile string) *ManagerActionCard {
	if health.Status != ActiveChildLaunchFailed {
		return nil
	}
	evidence := []string{
		fmt.Sprintf("stage: %s", firstNonEmpty(health.Stage, string(state.Workflow.CurrentNodeID))),
		fmt.Sprintf("child: %s", health.ChildID),
	}
	if health.PaneID != "" {
		evidence = append(evidence, fmt.Sprintf("pane: %s", health.PaneID))
	}
	if health.ExitCode != nil {
		evidence = append(evidence, fmt.Sprintf("exitCode: %d", *health.ExitCode))
	}
	if health.StatusPath != "" {
		evidence = append(evidence, fmt.Sprintf("status: %s", health.StatusPath))
	}
	for _, line := range health.OutputTail {
		evidence = append(evidence, "output tail: "+line)
	}
	if health.OutputPath != "" {
		evidence = append(evidence, "full output: "+health.OutputPath)
	}
	return &ManagerActionCard{
		Kind:              ActionChildLaunchFailed,
		Severity:          "error",
		Summary:           "child exited before qrspi_result",
		Evidence:          evidence,
		RecommendedAction: "clear failed child and relaunch same stage",
		SafeCommand:       fmt.Sprintf("vamos qrspi repair-state --state-file %s --clear-failed-child --relaunch", stateFile),
		ContinueCommand:   continueCommand(stateFile),
		RequiresHuman:     false,
	}
}
