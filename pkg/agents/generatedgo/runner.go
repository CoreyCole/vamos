package generatedgo

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultLogLimit = 64 * 1024

var defaultArtifactAllowlist = []string{"app.html", "results.csv", "manifest.json"}

type RunnerInput struct {
	WorkspaceDir      string
	OutputDir         string
	ModulePath        string
	CompileTimeout    time.Duration
	RunTimeout        time.Duration
	EnvAllowlist      map[string]string
	ArtifactAllowlist []string
	LogLimitBytes     int64
}

type RunnerResult struct {
	Status         BuildStatus
	Manifest       GeneratedManifest
	StdoutTail     string
	StderrTail     string
	SourceHash     string
	ArtifactHashes map[string]string
}

func BuildAndRun(ctx context.Context, input RunnerInput) (RunnerResult, error) {
	input, err := normalizeRunnerInput(input)
	if err != nil {
		return RunnerResult{}, err
	}
	if err := os.MkdirAll(input.OutputDir, 0o755); err != nil {
		return RunnerResult{}, fmt.Errorf("create output dir: %w", err)
	}
	sourceHash, err := HashSource(input.WorkspaceDir)
	if err != nil {
		return RunnerResult{}, err
	}

	binaryDir, err := os.MkdirTemp("", "vamos-generatedgo-*")
	if err != nil {
		return RunnerResult{}, fmt.Errorf("create binary temp dir: %w", err)
	}
	defer os.RemoveAll(binaryDir)
	binaryPath := filepath.Join(binaryDir, "generated-app")
	stdout, stderr := &tailBuffer{limit: logLimit(input)}, &tailBuffer{limit: logLimit(input)}
	env := runnerEnv(input.EnvAllowlist, input.OutputDir)

	compileCtx, compileCancel := context.WithTimeout(ctx, durationOrDefault(input.CompileTimeout, 30*time.Second))
	defer compileCancel()
	compileArgs := []string{"build", "-o", binaryPath, "."}
	if strings.TrimSpace(input.ModulePath) != "" && input.ModulePath != "." {
		compileArgs = []string{"build", "-o", binaryPath, input.ModulePath}
	}
	compileCmd := exec.CommandContext(compileCtx, "go", compileArgs...)
	compileCmd.Dir = input.WorkspaceDir
	compileCmd.Env = env
	compileCmd.Stdout = stdout
	compileCmd.Stderr = stderr
	if err := compileCmd.Run(); err != nil {
		return RunnerResult{Status: BuildStatusFailed, StdoutTail: stdout.String(), StderrTail: stderr.String(), SourceHash: sourceHash}, fmt.Errorf("compile generated Go: %w", err)
	}

	runCtx, runCancel := context.WithTimeout(ctx, durationOrDefault(input.RunTimeout, 30*time.Second))
	defer runCancel()
	runCmd := exec.CommandContext(runCtx, binaryPath)
	runCmd.Dir = input.WorkspaceDir
	runCmd.Env = env
	runCmd.Stdout = stdout
	runCmd.Stderr = stderr
	if err := runCmd.Run(); err != nil {
		return RunnerResult{Status: BuildStatusFailed, StdoutTail: stdout.String(), StderrTail: stderr.String(), SourceHash: sourceHash}, fmt.Errorf("run generated Go: %w", err)
	}

	if err := rejectUnsafeOutputs(input.OutputDir); err != nil {
		return RunnerResult{}, err
	}
	manifestPath := filepath.Join(input.OutputDir, "manifest.json")
	manifest, err := ValidateManifest(manifestPath, input.OutputDir)
	if err != nil {
		return RunnerResult{}, err
	}
	hashes, err := HashArtifacts(input.OutputDir, artifactAllowlist(input.ArtifactAllowlist))
	if err != nil {
		return RunnerResult{}, err
	}
	return RunnerResult{
		Status:         BuildStatusSucceeded,
		Manifest:       manifest,
		StdoutTail:     stdout.String(),
		StderrTail:     stderr.String(),
		SourceHash:     sourceHash,
		ArtifactHashes: hashes,
	}, nil
}

func normalizeRunnerInput(input RunnerInput) (RunnerInput, error) {
	if strings.TrimSpace(input.WorkspaceDir) == "" || strings.TrimSpace(input.OutputDir) == "" {
		return RunnerInput{}, fmt.Errorf("workspace and output dirs are required")
	}
	ws, err := filepath.Abs(input.WorkspaceDir)
	if err != nil {
		return RunnerInput{}, fmt.Errorf("resolve workspace dir: %w", err)
	}
	out, err := filepath.Abs(input.OutputDir)
	if err != nil {
		return RunnerInput{}, fmt.Errorf("resolve output dir: %w", err)
	}
	if ws == out || strings.HasPrefix(ws, out+string(os.PathSeparator)) {
		return RunnerInput{}, fmt.Errorf("workspace must not live under output dir")
	}
	input.WorkspaceDir = ws
	input.OutputDir = out
	return input, nil
}

func runnerEnv(allowed map[string]string, outputDir string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"GOCACHE=" + os.Getenv("GOCACHE"),
		"VAMOS_GENERATED_OUTPUT_DIR=" + outputDir,
	}
	for key, value := range allowed {
		if key == "" || strings.Contains(key, "=") {
			continue
		}
		env = append(env, key+"="+value)
	}
	return env
}

type tailBuffer struct {
	data  []byte
	limit int64
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.limit = defaultLogLimit
	}
	b.data = append(b.data, p...)
	if int64(len(b.data)) > b.limit {
		b.data = append([]byte(nil), b.data[len(b.data)-int(b.limit):]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string { return string(b.data) }

func logLimit(input RunnerInput) int64 {
	if input.LogLimitBytes > 0 {
		return input.LogLimitBytes
	}
	return defaultLogLimit
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func artifactAllowlist(allowlist []string) []string {
	if len(allowlist) == 0 {
		return defaultArtifactAllowlist
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(allowlist)+1)
	for _, item := range allowlist {
		item = filepath.ToSlash(filepath.Clean(strings.TrimSpace(item)))
		if item == "." || item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	if !seen["manifest.json"] {
		out = append(out, "manifest.json")
	}
	return out
}

func rejectUnsafeOutputs(outputDir string) error {
	outputRoot, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolve output dir: %w", err)
	}
	return filepath.WalkDir(outputRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("generated output contains symlink %s", path)
		}
		if !pathWithinRoot(path, outputRoot) {
			return fmt.Errorf("generated output escapes root %s", path)
		}
		return nil
	})
}

var _ io.Writer = (*tailBuffer)(nil)
