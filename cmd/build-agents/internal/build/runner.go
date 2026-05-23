package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type ExecRunner struct {
	repoRoot       string
	stdout, stderr io.Writer
}

func NewExecRunner(repoRoot string, stdout, stderr io.Writer) *ExecRunner {
	return &ExecRunner{repoRoot: repoRoot, stdout: stdout, stderr: stderr}
}

func (r *ExecRunner) Run(ctx context.Context, spec CommandSpec) error {
	if spec.Func != nil {
		return spec.Func(ctx, r.repoRoot, r)
	}
	if len(spec.Args) == 0 {
		return errors.New("empty command")
	}
	if cacheDir := spec.Env["GOCACHE"]; cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return fmt.Errorf("mkdir GOCACHE: %w", err)
		}
	}
	// #nosec G204 -- build step commands are fixed specs defined by this package.
	cmd := exec.CommandContext(ctx, spec.Args[0], spec.Args[1:]...)
	cmd.Dir = filepath.Join(r.repoRoot, filepath.FromSlash(spec.Dir))
	if spec.Quiet {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	} else {
		cmd.Stdout = r.stdout
		cmd.Stderr = r.stderr
	}
	cmd.Env = os.Environ()
	for key, value := range spec.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %v: %w", spec.Args, err)
	}
	return nil
}
